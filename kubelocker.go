package lockheed

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubeLocker struct {
	Clientset *kubernetes.Clientset
	Namespace string
	Prefix    string
}

func NewKubeLocker(cset *kubernetes.Clientset, namespace string) *KubeLocker {
	lock := &KubeLocker{
		Clientset: cset,
		Namespace: namespace,
		Prefix:    "lockheed",
	}
	return lock
}

func (locker *KubeLocker) GetConfigMapName(l *Lock) string {
	return locker.Prefix + "-" + l.Name
}

func (locker *KubeLocker) ConfigMapExists(l *Lock) (bool, error) {
	name := locker.GetConfigMapName(l)
	list, err := locker.Clientset.CoreV1().ConfigMaps(locker.Namespace).List(l.Context, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	found := false
	for _, item := range list.Items {
		if item.Name == name {
			found = true
			break
		}
	}
	return found, nil
}

func (locker *KubeLocker) CreateNewConfigMap(l *Lock) error {
	lockStateJson, err := json.Marshal(l)
	if err != nil {
		return err
	}
	cmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: locker.GetConfigMapName(l),
			Labels: map[string]string{
				"lockheed/lock": "",
			},
		},
		Data: map[string]string{
			"lock": string(lockStateJson),
		},
	}
	_, err = locker.Clientset.CoreV1().ConfigMaps(locker.Namespace).Create(l.Context, cmap, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (locker *KubeLocker) Init(l *Lock) error {
	exists, err := locker.ConfigMapExists(l)
	if err != nil {
		return err
	}
	if !exists {
		return locker.CreateNewConfigMap(l)
	}
	return nil
}

func (locker *KubeLocker) GetConfigMap(l *Lock) (*corev1.ConfigMap, error) {
	name := locker.GetConfigMapName(l)
	cmap, err := locker.Clientset.CoreV1().ConfigMaps(locker.Namespace).Get(l.Context, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cmap, nil
}

func (locker *KubeLocker) GetReservedConfigMap(l *Lock) (*corev1.ConfigMap, error) {
	attempt := 0
	for {
		attempt++
		cmap, err := locker.GetReservedConfigMapAttempt(l)
		if err == nil {
			return cmap, nil
		}
		if attempt >= 5 {
			return cmap, err
		}
		time.Sleep(time.Second)
	}
}

func (locker *KubeLocker) GetReservedConfigMapAttempt(l *Lock) (*corev1.ConfigMap, error) {
	name := locker.GetConfigMapName(l)
	cmap, err := locker.GetConfigMap(l)
	if err != nil {
		return nil, err
	}
	if val, reserved := cmap.ObjectMeta.Annotations["reserved/by"]; reserved {
		expires, err := time.Parse(time.RFC3339, cmap.ObjectMeta.Annotations["reserved/expires"])
		if err != nil {
			return nil, err
		}
		if time.Now().Before(expires) {
			return nil, fmt.Errorf("ConfigMap %s reserved by %s", name, val)
		}
	}
	if cmap.ObjectMeta.Annotations == nil {
		cmap.ObjectMeta.Annotations = make(map[string]string)
	}
	cmap.ObjectMeta.Annotations["reserved/by"] = l.InstanceID
	cmap.ObjectMeta.Annotations["reserved/expires"] = time.Now().Add(30 * time.Second).Format(time.RFC3339)
	cmap, err = locker.Clientset.CoreV1().ConfigMaps(locker.Namespace).Update(l.Context, cmap, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("Error setting reservation for %s: %w", name, err)
	}
	return cmap, nil
}

func (locker *KubeLocker) UpdateAndReleaseConfigMap(ctx context.Context, cmap *corev1.ConfigMap) error {
	delete(cmap.ObjectMeta.Annotations, "reserved/by")
	delete(cmap.ObjectMeta.Annotations, "reserved/expires")
	_, err := locker.Clientset.CoreV1().ConfigMaps(locker.Namespace).Update(ctx, cmap, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

// TODO: look at potential corner-cases
func (locker *KubeLocker) ReleaseConfigMap(l *Lock) error {
	cmap, err := locker.GetConfigMap(l)
	if err != nil {
		return err
	}
	if cmap.ObjectMeta.Annotations["reserved/by"] == l.InstanceID {
		delete(cmap.ObjectMeta.Annotations, "reserved/by")
		delete(cmap.ObjectMeta.Annotations, "reserved/expires")
		locker.Clientset.CoreV1().ConfigMaps(locker.Namespace).Update(l.Context, cmap, metav1.UpdateOptions{})
	}
	return nil
}

func (locker *KubeLocker) GetAllLocks(ctx context.Context) ([]*Lock, error) {
	var result []*Lock
	opts := metav1.ListOptions{
		LabelSelector: "lockheed/lock",
	}
	list, err := locker.Clientset.CoreV1().ConfigMaps(locker.Namespace).List(context.Background(), opts)
	if err != nil {
		return result, err
	}
	for _, item := range list.Items {
		lockState := &Lock{}
		if err := json.Unmarshal([]byte(item.Data["lock"]), lockState); err != nil {
			return result, err
		}
		result = append(result, lockState)
	}
	return result, nil
}

func (locker *KubeLocker) Acquire(ctx context.Context, l *Lock) error {
	err := locker.Init(l)
	if err != nil {
		return fmt.Errorf("Error initiating lock: %w", err)
	}
	cmap, err := locker.GetReservedConfigMap(l)
	if err != nil {
		return fmt.Errorf("GetReservedConfigMap: %w", err)
	}

	lockState := &Lock{}
	if err := json.Unmarshal([]byte(cmap.Data["lock"]), lockState); err != nil {
		locker.ReleaseConfigMap(l)
		return err
	}

	force := false
	if l.forceCondition != nil {
		force, err = lockState.Evaluate(l.forceCondition)
		if err != nil {
			return err
		}
	}

	if lockState.LockType == "" {
		lockState.LockType = LockTypeMutex
	}

	leaseCount := len(lockState.Leases)
	if lockState.LockType == LockTypeMutex && leaseCount > 0 {
		if leaseCount > 1 {
			locker.ReleaseConfigMap(l)
			return fmt.Errorf("Invalid number of leases for mutex lock: %d", leaseCount)
		}
		for key, lease := range lockState.Leases {
			if key != l.InstanceID && !lease.Expired() && !force {
				locker.ReleaseConfigMap(l)
				return fmt.Errorf("Mutex lock is already held by %s", lease.InstanceID)
			}
		}
	}

	if lockState.LockType == LockTypeMutex {
		lockState.Leases = map[string]LockLease{
			l.InstanceID: LockLease{InstanceID: l.InstanceID, Expires: l.NewExpiryTime()},
		}
	} else {
		locker.ReleaseConfigMap(l)
		return fmt.Errorf("Non-mutex locks not implemented yet")
	}

	syncLockFields(l, lockState)

	lockStateJson, err := json.Marshal(lockState)
	if err != nil {
		locker.ReleaseConfigMap(l)
		return err
	}
	cmap.Data["lock"] = string(lockStateJson)

	return locker.UpdateAndReleaseConfigMap(l.Context, cmap)
}

func (locker *KubeLocker) Renew(ctx context.Context, l *Lock) error {
	cmap, err := locker.GetReservedConfigMap(l)
	if err != nil {
		return err
	}

	lockState := &Lock{}
	if err := json.Unmarshal([]byte(cmap.Data["lock"]), lockState); err != nil {
		return err
	}

	lease, exists := lockState.Leases[l.InstanceID]
	if !exists {
		return fmt.Errorf("No lease to renew for %s", l.InstanceID)
	}
	if lease.Expired() {
		return fmt.Errorf("Lease on lock %s for %s already expired", l.Name, l.InstanceID)
	}
	lease.Expires = l.NewExpiryTime()
	lockState.Leases[l.InstanceID] = lease

	lockStateJson, err := json.Marshal(lockState)
	if err != nil {
		return err
	}
	cmap.Data["lock"] = string(lockStateJson)

	if err := locker.UpdateAndReleaseConfigMap(l.Context, cmap); err != nil {
		return err
	}
	return nil
}

func (locker *KubeLocker) Release(ctx context.Context, l *Lock) error {
	cmap, err := locker.GetReservedConfigMap(l)
	if err != nil {
		return err
	}

	lockState := &Lock{}
	if err := json.Unmarshal([]byte(cmap.Data["lock"]), lockState); err != nil {
		return err
	}

	delete(lockState.Leases, l.InstanceID)
	syncLockFields(l, lockState)

	lockStateJson, err := json.Marshal(lockState)
	if err != nil {
		return err
	}
	cmap.Data["lock"] = string(lockStateJson)

	return locker.UpdateAndReleaseConfigMap(l.Context, cmap)
}

func GetKubeConfig() *rest.Config {
	var config *rest.Config
	var kubeconfig *string
	var err error
	// creates the in-cluster config
	config, err = rest.InClusterConfig()
	if err != nil {
		if kubeconfig == nil {
			if home := os.Getenv("HOME"); home != "" {
				kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
			} else {
				kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
			}
			flag.Parse()
		}
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	}
	return config
}

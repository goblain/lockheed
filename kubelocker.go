package lockheed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
	lockStateJson, err := json.Marshal(&LockState{LockType: LockTypeMutex})
	if err != nil {
		return err
	}
	cmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: locker.GetConfigMapName(l),
			Labels: map[string]string{
				"lockheed/lockstate": "",
			},
		},
		Data: map[string]string{
			"lockState": string(lockStateJson),
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

func (locker *KubeLocker) List() ([]string, error) {
	return []string{}, nil
}

func (locker *KubeLocker) Acquire(l *Lock) error {
	locker.Init(l)
	cmap, err := locker.GetReservedConfigMap(l)
	if err != nil {
		return err
	}

	lockState := &LockState{}
	if err := json.Unmarshal([]byte(cmap.Data["lockState"]), lockState); err != nil {
		return err
	}

	leaseCount := len(lockState.Leases)
	if lockState.LockType == LockTypeMutex && leaseCount > 0 {
		if leaseCount > 1 {
			return fmt.Errorf("Invalid number of leases for mutex lock: %d", leaseCount)
		}
		lease, exists := lockState.Leases[l.InstanceID]
		if exists && time.Now().Before(lease.Expires) {
			return fmt.Errorf("Mutex lock is already held by %s", lease.InstanceID)
		}
	}

	if lockState.LockType == "" || lockState.LockType == LockTypeMutex {
		lockState.LockType = LockTypeMutex
		lockState.Leases = map[string]LockLease{
			l.InstanceID: LockLease{InstanceID: l.InstanceID, Expires: l.NewExpiryTime()},
		}
	} else {
		return fmt.Errorf("Non-mutex locks not implemented yet")
	}

	lockStateJson, err := json.Marshal(lockState)
	if err != nil {
		return err
	}
	cmap.Data["lockState"] = string(lockStateJson)

	return locker.UpdateAndReleaseConfigMap(l.Context, cmap)
}

func (locker *KubeLocker) Renew(l *Lock) error {
	cmap, err := locker.GetReservedConfigMap(l)
	if err != nil {
		return err
	}

	lockState := &LockState{}
	if err := json.Unmarshal([]byte(cmap.Data["lockState"]), lockState); err != nil {
		return err
	}

	lease, exists := lockState.Leases[l.InstanceID]
	if !exists {
		return fmt.Errorf("No lease to renew for %s", l.InstanceID)
	}
	if time.Now().After(lease.Expires) {
		return fmt.Errorf("Lease on lock %s for %s already expired", l.Name, l.InstanceID)
	}
	lease.Expires = l.NewExpiryTime()
	lockState.Leases[l.InstanceID] = lease

	lockStateJson, err := json.Marshal(lockState)
	if err != nil {
		return err
	}
	cmap.Data["lockState"] = string(lockStateJson)

	if err := locker.UpdateAndReleaseConfigMap(l.Context, cmap); err != nil {
		return err
	}
	return nil
}

func (locker *KubeLocker) Release(l *Lock) error {
	cmap, err := locker.GetReservedConfigMap(l)
	if err != nil {
		return err
	}

	lockState := &LockState{}
	if err := json.Unmarshal([]byte(cmap.Data["lockState"]), lockState); err != nil {
		return err
	}

	delete(lockState.Leases, l.InstanceID)

	lockStateJson, err := json.Marshal(lockState)
	if err != nil {
		return err
	}
	cmap.Data["lockState"] = string(lockStateJson)

	return locker.UpdateAndReleaseConfigMap(l.Context, cmap)
}

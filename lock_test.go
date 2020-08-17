package lockheed

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var cset *kubernetes.Clientset

func GetTestKubeLocker() LockerInterface {
	if cset == nil {
		cset, _ = kubernetes.NewForConfig(GetKubeConfig())
	}
	return NewKubeLocker(cset, "default")
}

func CreateEmptyConfigmap() error {
	ctx := context.Background()
	if cset == nil {
		cset, _ = kubernetes.NewForConfig(GetKubeConfig())
	}
	cmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lockheed-empty",
		},
	}
	_, err := cset.CoreV1().ConfigMaps("default").Create(ctx, cmap, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil

}

func TestKubeLockerForce(t *testing.T) {
	lockA := NewLock("testlockx", GetTestKubeLocker()).WithDuration(5 * time.Second).WithTags([]string{"forceme"})
	if err := lockA.Acquire(); err != nil {
		t.Error(err)
	}
	lockB := NewLock("testlockx", GetTestKubeLocker()).WithDuration(5 * time.Second)
	if err := lockB.Acquire(); err == nil {
		t.Error("Failure expected")
	}

	lockC := NewLock("testlockx", GetTestKubeLocker()).
		WithDuration(30 * time.Second).
		WithResetTags().
		WithForce(Condition{
			Operation: OperationContains,
			Field:     FieldTags,
			Value:     "forceme",
		})
	if err := lockC.Acquire(); err != nil {
		t.Error(err)
	}
	lockD := NewLock("testlockx", GetTestKubeLocker()).
		WithDuration(30 * time.Second).
		WithResetTags().
		WithForce(Condition{
			Operation: OperationContains,
			Field:     FieldTags,
			Value:     "forceme",
		})
	if err := lockD.Acquire(); err == nil {
		t.Error("Failure expected")
	}
}

func TestKubeLocker(t *testing.T) {
	CreateEmptyConfigmap()
	lockA := NewLock("testlock", GetTestKubeLocker()).
		WithDuration(10 * time.Second).
		WithRenewInterval(5 * time.Second).
		WithTags([]string{"testtag"})
	if err := lockA.Acquire(); err != nil {
		t.Error(err)
	}
	lockB := NewLock("testlock", GetTestKubeLocker()).
		WithDuration(10 * time.Second).
		WithRenewInterval(5 * time.Second).
		WithTags([]string{"testtag"})
	if err := lockB.AcquireRetry(2, 2); err == nil {
		t.Error("Expected to fail")
	}
	lockC := NewLock("testlock2", GetTestKubeLocker()).
		WithDuration(10 * time.Second).
		WithRenewInterval(5 * time.Second).
		WithTags([]string{"testtag"})
	if err := lockC.AcquireRetry(5, 3); err != nil {
		t.Error(err)
	}
	if err := lockA.Release(); err != nil {
		t.Error(err)
	}
	if err := lockB.Acquire(); err != nil {
		t.Error(err)
	}
	if err := lockB.Release(); err != nil {
		t.Error(err)
	}

	cond := &Condition{
		Operation: OperationAnd,
		Conditions: &[]Condition{
			Condition{
				Operation: OperationEquals,
				Field:     FieldAcquired,
				Value:     true,
			},
			Condition{
				Operation: OperationContains,
				Field:     FieldTags,
				Value:     "testtag",
			},
		},
	}

	locks, err := GetLocks(NewKubeLocker(cset, "default"), cond)
	if err != nil {
		t.Error(err)
	}
	if len(locks) != 1 || locks[0].Name != "testlock2" {
		t.Error("Lock not listed as expected")
	}

	if err := lockC.Release(); err != nil {
		t.Error(err)
	}
	if err := lockC.Release(); err != nil {
		t.Error(err)
	}
	time.Sleep(2 * time.Second)
}

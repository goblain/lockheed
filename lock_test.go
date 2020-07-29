package lockheed

import (
	"context"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
)

func TestKubeLocker(t *testing.T) {
	cset, err := kubernetes.NewForConfig(GetKubeConfig())
	if err != nil {
		t.Error(err)
	}

	ctx := context.Background()
	// ctx, cancel := context.WithCancel(context.Background())
	opts := Options{
		Duration:      30 * time.Second,
		RenewInterval: 5 * time.Second,
		Tags:          []string{"testtag"},
	}
	lockA := NewLock("testlock", ctx, NewKubeLocker(cset, "default"), opts)
	if err := lockA.Acquire(); err != nil {
		t.Error(err)
	}
	lockB := NewLock("testlock", ctx, NewKubeLocker(cset, "default"), opts)
	if err := lockB.AcquireRetry(2, 2); err == nil {
		t.Error("Expected to fail")
	}
	lockC := NewLock("testlock2", ctx, NewKubeLocker(cset, "default"), opts)
	if err := lockC.AcquireRetry(2, 2); err != nil {
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

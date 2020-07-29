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
		RenewInterval: 4 * time.Second,
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
	if err := lockC.Release(); err != nil {
		t.Error(err)
	}
	if err := lockC.Release(); err != nil {
		t.Error(err)
	}
	time.Sleep(2 * time.Second)
}

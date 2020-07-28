package lockheed

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func TestKubeLocker(t *testing.T) {
	cset, err := kubernetes.NewForConfig(GetConfig())
	if err != nil {
		t.Error(err)
	}

	ctx := context.Background()
	// ctx, cancel := context.WithCancel(context.Background())
	opts := Options{
		Duration:      30 * time.Second,
		RenewInterval: 4 * time.Second,
	}
	lock := NewLock("testlock", ctx, NewKubeLocker(cset, "default"), opts)
	if err := lock.Acquire(); err != nil {
		t.Error(err)
	}
	time.Sleep(10 * time.Second)
	lock.Release()
	// cancel()
	time.Sleep(10 * time.Second)
}

func GetConfig() *rest.Config {
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

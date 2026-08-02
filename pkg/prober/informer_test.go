package prober_test

import (
	"context"
	"testing"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/demirdilek/kube-prober/pkg/prober"
)

func TestTargetWatcher_InformerEvents(t *testing.T) {
	// 1. Create dummy EndpointSlice BEFORE starting the Informer
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service-slice",
			Namespace: "default",
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.244.0.5"},
			},
		},
	}

	// Initialize Fake Clientset with the initial object
	clientset := fake.NewSimpleClientset(slice)
	registry := prober.NewRegistry()
	watcher := prober.NewTargetWatcher(clientset, registry)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 2. Start watcher in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- watcher.Start(ctx)
	}()

	// 3. Wait until the initial object is processed by the Informer
	var targets []string
	assertEventually(t, 1*time.Second, 50*time.Millisecond, func() bool {
		targets = registry.GetTargets()
		return len(targets) > 0
	}, "expected targets after EndpointSlice creation")

	// 4. Test DELETE event
	err := clientset.DiscoveryV1().EndpointSlices("default").Delete(context.Background(), slice.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete EndpointSlice: %v", err)
	}

	assertEventually(t, 1*time.Second, 50*time.Millisecond, func() bool {
		targets = registry.GetTargets()
		return len(targets) == 0
	}, "expected 0 targets after EndpointSlice deletion")
}

// Helper to poll without sleeping fixed amounts of time
func assertEventually(t *testing.T, timeout, interval time.Duration, condition func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatalf("timeout reached waiting for condition: %s", msg)
}

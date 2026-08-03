package prober

import (
	"fmt"
	"sync"
	"testing"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRegistry_UpdateAndRemoveEndpointSlice(t *testing.T) {
	registry := NewRegistry()

	port8080 := int32(8080)
	sliceWithPort := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slice-1",
			Namespace: "default",
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: &port8080},
		},
		Endpoints: []discoveryv1.Endpoint{
			{Addresses: []string{"10.244.0.5", "10.244.0.6"}},
		},
	}

	registry.UpdateFromEndpointSlice(sliceWithPort)

	targets := registry.GetTargets()
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	expectedURL1 := "http://10.244.0.5:8080/healthz"
	expectedURL2 := "http://10.244.0.6:8080/healthz"

	if !Contains(targets, expectedURL1) || !Contains(targets, expectedURL2) {
		t.Errorf("expected targets to contain %s and %s, got %v", expectedURL1, expectedURL2, targets)
	}

	sliceDefaultPort := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slice-2",
			Namespace: "default",
		},
		Ports:     []discoveryv1.EndpointPort{},
		Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.244.0.10"}}},
	}

	registry.UpdateFromEndpointSlice(sliceDefaultPort)
	targets = registry.GetTargets()
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}

	expectedDefaultURL := "http://10.244.0.10:80/healthz"
	if !Contains(targets, expectedDefaultURL) {
		t.Errorf("expected targets to contain fallback URL %s", expectedDefaultURL)
	}

	registry.RemoveEndpointSlice(sliceWithPort)
	targets = registry.GetTargets()

	if len(targets) != 1 {
		t.Fatalf("expected 1 target remaining, got %d", len(targets))
	}

	if Contains(targets, expectedURL1) {
		t.Errorf("did not expect registry to contain removed target %s", expectedURL1)
	}
}

func TestRegistry_ConcurrencySafety(t *testing.T) {
	registry := NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(2)
		ip := fmt.Sprintf("10.244.0.%d", i)

		go func(addr string) {
			defer wg.Done()
			port := int32(80)
			slice := &discoveryv1.EndpointSlice{
				Ports:     []discoveryv1.EndpointPort{{Port: &port}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{addr}}},
			}
			registry.UpdateFromEndpointSlice(slice)
		}(ip)

		go func() {
			defer wg.Done()
			_ = registry.GetTargets()
		}()
	}

	wg.Wait()

	targets := registry.GetTargets()
	if len(targets) != 20 {
		t.Errorf("expected 20 concurrent targets registered, got %d", len(targets))
	}
}
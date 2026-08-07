package prober

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRegistry_ShardingRendezvousHashing(t *testing.T) {
	registry := NewRegistry()

	// 1. Simulate a cluster topology with 3 prober replicas
	podIPs := []string{"10.244.0.100", "10.244.0.101", "10.244.0.102"}
	selfIP := "10.244.0.100"

	// Configure the registry with the mocked HPA topology
	registry.SetSelfIP(selfIP)
	registry.UpdatePeers(podIPs)

	// 2. Define a list of discovered target URLs
	targets := []string{
		"http://10.244.0.1:8080/healthz",
		"http://10.244.0.2:8080/healthz",
		"http://10.244.0.3:8080/healthz",
		"http://10.244.0.4:8080/healthz",
		"http://10.244.0.5:8080/healthz",
		"http://10.244.0.6:8080/healthz",
		"http://10.244.0.7:8080/healthz",
		"http://10.244.0.8:8080/healthz",
	}

	// 3. Count how many targets are assigned to THIS specific pod
	assignedToSelf := 0
	for _, target := range targets {
		if registry.ShouldProcessTarget(target) {
			assignedToSelf++
		}
	}

	// 4. Verify that not all targets are assigned to a single pod (sharding is active)
	// With 8 targets and 3 pods, it is statistically highly probable to get a subset.
	if assignedToSelf == 0 || assignedToSelf == len(targets) {
		t.Errorf("expected distributed targets, but got %d assigned to self out of %d", assignedToSelf, len(targets))
	}
}

func TestRegistry_UpdateAndRemoveEndpointSlice(t *testing.T) {
	registry := NewRegistry()

	port8080 := int32(8080)
	
	// Here is where sliceWithPort is defined
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

	// 1. Test Add with custom path and explicit http scheme
	customPath := "/custom-health"
	registry.UpdateFromEndpointSlice(sliceWithPort, "http", customPath)

	targets := registry.GetTargets()
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	expectedURL1 := "http://10.244.0.5:8080/custom-health"
	expectedURL2 := "http://10.244.0.6:8080/custom-health"

	if !slices.Contains(targets, expectedURL1) || !slices.Contains(targets, expectedURL2) {
		t.Errorf("expected targets to contain %s and %s, got %v", expectedURL1, expectedURL2, targets)
	}

	// Verify events were emitted
	if len(registry.Events) != 2 {
		t.Fatalf("expected 2 events in channel, got %d", len(registry.Events))
	}

	// 2. Test Idempotency (adding the same targets again should not duplicate events)
	registry.UpdateFromEndpointSlice(sliceWithPort, "http", customPath)
	if len(registry.Events) != 2 {
		t.Errorf("expected 2 events total (0 new), got %d", len(registry.Events))
	}

	// 3. Test Remove with custom path
	registry.RemoveEndpointSlice(sliceWithPort, "http", customPath)
	targets = registry.GetTargets()

	if len(targets) != 0 {
		t.Fatalf("expected 0 targets remaining, got %d", len(targets))
	}
}

func TestRegistry_ConcurrencySafety(t *testing.T) {
	registry := NewRegistry()
	var wg sync.WaitGroup

	// Simulate 20 concurrent discoveries
	for i := 0; i < 20; i++ {
		wg.Add(2)
		ip := fmt.Sprintf("10.244.0.%d", i)

		go func(addr string) {
			defer wg.Done()
			port := int32(80)
			
			// Here is where the slice is defined inside the goroutine
			slice := &discoveryv1.EndpointSlice{
				Ports:     []discoveryv1.EndpointPort{{Port: &port}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{addr}}},
			}
			
			// Updated signature passing the "http" scheme
			registry.UpdateFromEndpointSlice(slice, "http", "/healthz")
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
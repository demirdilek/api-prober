package prober

import (
	"context"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestKubeWatcher_InformerEvents_DynamicPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientset := fake.NewSimpleClientset()
	registry := NewRegistry()

	// 1. Create a fake service with a custom probe/path annotation
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
			Annotations: map[string]string{
				"probe/path": "/custom-metrics",
			},
		},
	}
	_, err := clientset.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create fake service: %v", err)
	}

	watcher := NewKubeWatcher(clientset, registry)

	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	endpointSliceInformer := informerFactory.Discovery().V1().EndpointSlices()

	_, err = endpointSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// In TestKubeWatcher_InformerEvents_DynamicPath:
		AddFunc: func(obj interface{}) {
			if slice, ok := obj.(*discoveryv1.EndpointSlice); ok {
				// Parse both scheme and path to satisfy the new UpdateFromEndpointSlice signature
				scheme, path := watcher.getProbeSchemeAndPath(ctx, slice)
				registry.UpdateFromEndpointSlice(slice, scheme, path)
			}
		},
	})

	if err != nil {
		t.Fatalf("failed to add event handler to informer: %v", err)
	}

	informerFactory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), endpointSliceInformer.Informer().HasSynced) {
		t.Fatal("timed out waiting for informer caches to sync")
	}

	// 2. Create an EndpointSlice pointing to the service
	port8080 := int32(8080)
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slice",
			Namespace: "default",
			Labels: map[string]string{
				"kubernetes.io/service-name": "test-service",
				"probe":                      "true",
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: &port8080},
		},
		Endpoints: []discoveryv1.Endpoint{
			{Addresses: []string{"10.244.0.15"}},
		},
	}

	_, err = clientset.DiscoveryV1().EndpointSlices("default").Create(ctx, slice, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create fake endpoint slice: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	targets := registry.GetTargets()
	if len(targets) != 1 {
		t.Fatalf("expected 1 target discovered via informer, got %d", len(targets))
	}

	// 3. Verify that the path was DYNAMICALLY extracted from the annotation
	expectedURL := "http://10.244.0.15:8080/custom-metrics"
	if !slices.Contains(targets, expectedURL) {
		t.Errorf("expected target %s to be registered dynamically, got %v", expectedURL, targets)
	}
}

func TestKubeWatcher_WatchPeers(t *testing.T) {
	// 1. Set up a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Initialize the fake clientset and registry
	fakeClientset := fake.NewSimpleClientset()
	reg := NewRegistry()
	reg.SetSelfIP("10.244.0.100") // Simulate our local pod IP

	// 3. Start the peer watcher asynchronously using KubeWatcher
	watcher := NewKubeWatcher(fakeClientset, reg)
	go watcher.WatchPeers(ctx)

	// Allow the shared informer a brief moment to start and sync its cache
	time.Sleep(100 * time.Millisecond)

	// 4. Simulate HPA scaling: Dynamically create a new EndpointSlice
	ready := true
	mockSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-prober-slice-1",
			Namespace: "default",
			Labels: map[string]string{
				"kubernetes.io/service-name": "kube-prober",
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.244.0.100", "10.244.0.101"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: &ready,
				},
			},
		},
	}

	// Inject the slice into the fake cluster
	_, err := fakeClientset.DiscoveryV1().EndpointSlices("default").Create(ctx, mockSlice, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create mock EndpointSlice: %v", err)
	}

	// 5. Allow the informer time to process the event
	time.Sleep(200 * time.Millisecond)

	// 6. Assertions (Verify the state actually changed)
	peers := reg.GetPeers()
	if len(peers) != 2 {
		t.Errorf("Expected 2 peers in the registry, but got %d", len(peers))
	}
	
	// Because IPs are sorted by the registry, 10.244.0.100 will always be at index 0
	if peers[0] != "10.244.0.100" || peers[1] != "10.244.0.101" {
		t.Errorf("Unexpected peer sorting or IPs: %v", peers)
	}
}
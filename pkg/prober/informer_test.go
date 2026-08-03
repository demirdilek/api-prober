package prober

import (
	"context"
	"testing"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

// English comments as requested

func TestTargetWatcher_InformerEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientset := fake.NewSimpleClientset()
	registry := NewRegistry()

	// Use informer factory with resync period to properly sync caches in tests
	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	endpointSliceInformer := informerFactory.Discovery().V1().EndpointSlices()

	endpointSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if slice, ok := obj.(*discoveryv1.EndpointSlice); ok {
				registry.UpdateFromEndpointSlice(slice)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if slice, ok := newObj.(*discoveryv1.EndpointSlice); ok {
				registry.UpdateFromEndpointSlice(slice)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if slice, ok := obj.(*discoveryv1.EndpointSlice); ok {
				registry.RemoveEndpointSlice(slice)
			}
		},
	})

	informerFactory.Start(ctx.Done())

	// Wait explicitly for caches to sync
	if !cache.WaitForCacheSync(ctx.Done(), endpointSliceInformer.Informer().HasSynced) {
		t.Fatal("timed out waiting for informer caches to sync")
	}

	port8080 := int32(8080)
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slice",
			Namespace: "default",
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: &port8080},
		},
		Endpoints: []discoveryv1.Endpoint{
			{Addresses: []string{"10.244.0.15"}},
		},
	}

	_, err := clientset.DiscoveryV1().EndpointSlices("default").Create(ctx, slice, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create fake endpoint slice: %v", err)
	}

	// Give event handler a brief moment to process event
	time.Sleep(100 * time.Millisecond)

	targets := registry.GetTargets()
	if len(targets) != 1 {
		t.Fatalf("expected 1 target discovered via informer, got %d", len(targets))
	}

	expectedURL := "http://10.244.0.15:8080/healthz"
	if !Contains(targets, expectedURL) {
		t.Errorf("expected target %s to be registered, got %v", expectedURL, targets)
	}
}
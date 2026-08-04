package prober

import (
	"context"
	"testing"
	"time"
	"slices"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestTargetWatcher_InformerEvents_DynamicPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientset := fake.NewSimpleClientset()
	registry := NewRegistry()

	// 1. Erstelle einen Fake Service mit custom probe/path Annotation
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

	watcher := NewTargetWatcher(clientset, registry)

	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	endpointSliceInformer := informerFactory.Discovery().V1().EndpointSlices()

	endpointSliceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if slice, ok := obj.(*discoveryv1.EndpointSlice); ok {
				path := watcher.getProbePath(ctx, slice)
				registry.UpdateFromEndpointSlice(slice, path)
			}
		},
	})

	informerFactory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), endpointSliceInformer.Informer().HasSynced) {
		t.Fatal("timed out waiting for informer caches to sync")
	}

	// 2. Erstelle ein EndpointSlice, das auf das Service verweist
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

	// 3. Verifiziere, dass der Pfad DYNAMISCH aus der Annotation ausgelesen wurde!
	expectedURL := "http://10.244.0.15:8080/custom-metrics"
	if !slices.Contains(targets, expectedURL) {
		t.Errorf("expected target %s to be registered dynamically, got %v", expectedURL, targets)
	}
}
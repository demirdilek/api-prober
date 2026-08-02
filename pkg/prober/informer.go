package prober

import (
	"context"
	"fmt"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type TargetWatcher struct {
	clientset kubernetes.Interface
	registry  *Registry
}

func NewTargetWatcher(clientset kubernetes.Interface, reg *Registry) *TargetWatcher {
	return &TargetWatcher{
		clientset: clientset,
		registry:  reg,
	}
}

func (w *TargetWatcher) Start(ctx context.Context) error {
	factory := informers.NewSharedInformerFactoryWithOptions(
		w.clientset,
		10*time.Minute,
	)

	// Modern EndpointSlice Informer (Kubernetes 1.21+ / replacement for corev1.Endpoints)
	endpointSliceInformer := factory.Discovery().V1().EndpointSlices().Informer()

	_, err := endpointSliceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if slice, ok := obj.(*discoveryv1.EndpointSlice); ok {
				w.registry.UpdateFromEndpointSlice(slice)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if newSlice, ok := newObj.(*discoveryv1.EndpointSlice); ok {
				w.registry.UpdateFromEndpointSlice(newSlice)
			}
		},
		DeleteFunc: func(obj interface{}) {
			slice, ok := obj.(*discoveryv1.EndpointSlice)
			if !ok {
				// Handle tombstone state if object was deleted while offline
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return
				}
				slice, ok = tombstone.Obj.(*discoveryv1.EndpointSlice)
				if !ok {
					return
				}
			}
			w.registry.RemoveEndpointSlice(slice)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler to EndpointSlice informer: %w", err)
	}

	factory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), endpointSliceInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer cache: %w", ctx.Err())
	}

	return nil
}

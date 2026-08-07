package prober

import (
	"context"
	"fmt"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// KubeWatcher observes K8s resources (EndpointSlices) to keep the target 
// registry in sync and maintain the active replica topology for sharding.
type KubeWatcher struct {
    clientset kubernetes.Interface
    registry  *Registry
}

func NewKubeWatcher(clientset kubernetes.Interface, reg *Registry) *KubeWatcher {
	return &KubeWatcher{
		clientset: clientset,
		registry:  reg,
	}
}

func (w *KubeWatcher) getProbePath(ctx context.Context, slice *discoveryv1.EndpointSlice) string {
	svcName := slice.Labels["kubernetes.io/service-name"]
	if svcName == "" {
		return "/healthz"
	}

	svc, err := w.clientset.CoreV1().Services(slice.Namespace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil || svc.Annotations == nil {
		return "/healthz"
	}

	if path, exists := svc.Annotations["probe/path"]; exists && path != "" {
		return path
	}

	return "/healthz"
}

func (w *KubeWatcher) Start(ctx context.Context) error {
	factory := informers.NewSharedInformerFactoryWithOptions(
		w.clientset,
		10*time.Minute,
	)

	endpointSliceInformer := factory.Discovery().V1().EndpointSlices().Informer()

	_, err := endpointSliceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if slice, ok := obj.(*discoveryv1.EndpointSlice); ok {
				if slice.Labels["probe"] == "true" {
					path := w.getProbePath(ctx, slice)
					w.registry.UpdateFromEndpointSlice(slice, path)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if newSlice, ok := newObj.(*discoveryv1.EndpointSlice); ok {
				path := w.getProbePath(ctx, newSlice)
				if newSlice.Labels["probe"] == "true" {
					w.registry.UpdateFromEndpointSlice(newSlice, path)
				} else {
					w.registry.RemoveEndpointSlice(newSlice, path)
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			slice, ok := obj.(*discoveryv1.EndpointSlice)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return
				}
				slice, ok = tombstone.Obj.(*discoveryv1.EndpointSlice)
				if !ok {
					return
				}
			}
			if slice.Labels["probe"] == "true" {
				path := w.getProbePath(ctx, slice)
				w.registry.RemoveEndpointSlice(slice, path)
			}
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

// WatchPeers observes EndpointSlices of the prober deployment itself
// to keep the active replica topology synced for Rendezvous Hashing.
func (w *KubeWatcher) WatchPeers(ctx context.Context) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		w.clientset,
		10*time.Minute,
		informers.WithNamespace("default"), // Adjust if deployed to a different namespace
	)

	informer := factory.Discovery().V1().EndpointSlices().Informer()

	updatePeers := func() {
		var peerIPs []string
		
		for _, obj := range informer.GetStore().List() {
			slice, ok := obj.(*discoveryv1.EndpointSlice)
			
			if ok && slice.Labels["kubernetes.io/service-name"] == "kube-prober" {
				for _, ep := range slice.Endpoints {
					if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
						peerIPs = append(peerIPs, ep.Addresses...)
					}
				}
			}
		}
		
		if len(peerIPs) > 0 {
			w.registry.UpdatePeers(peerIPs)
		}
	}

	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { updatePeers() },
		UpdateFunc: func(oldObj, newObj interface{}) { updatePeers() },
		DeleteFunc: func(obj interface{}) { updatePeers() },
	})

	factory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	
	updatePeers()
}
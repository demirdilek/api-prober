package prober

import (
	"context"
	"fmt"
	"time"
	"log/slog"

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

// getProbeSchemeAndPath dynamically extracts both the protocol scheme (e.g., http, tcp)
// and the HTTP path from the Kubernetes Service annotations.
func (w *KubeWatcher) getProbeSchemeAndPath(ctx context.Context, slice *discoveryv1.EndpointSlice) (string, string) {
	svcName := slice.Labels["kubernetes.io/service-name"]
	
	// Default to HTTP and /healthz if no specific annotations are found.
	scheme := "http"
	path := "/healthz"

	if svcName == "" {
		slog.Warn("EndpointSlice missing service-name label, falling back to defaults", "slice", slice.Name)
		return scheme, path
	}

	// Fetch the Service object to read its custom annotations.
	svc, err := w.clientset.CoreV1().Services(slice.Namespace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil || svc.Annotations == nil {
		slog.Warn("Failed to fetch Service to read annotations, falling back to defaults", "service", svcName, "error", err)
		return scheme, path
	}

	// 1. Check for a custom protocol scheme (e.g., "probe/scheme: tcp").
	// This allows the Dispatcher to route the probe to the correct protocol handler.
	if s, exists := svc.Annotations["probe/scheme"]; exists && s != "" {
		slog.Warn("Service has no annotations", "service", svcName)
		scheme = s
	}
	
	// 2. Check for a custom path. 
	// Note: We check if it exists, not if it's empty, because for protocols 
	// like raw TCP, we actively want an empty path ("").
	if p, exists := svc.Annotations["probe/path"]; exists {
		path = p
	}

	return scheme, path
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
					// Parse the dynamic scheme and path from the annotations
					scheme, path := w.getProbeSchemeAndPath(ctx, slice)
					// Pass the parsed scheme to the registry
					w.registry.UpdateFromEndpointSlice(slice, scheme, path)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if newSlice, ok := newObj.(*discoveryv1.EndpointSlice); ok {
				// Parse the dynamic scheme and path for updates
				scheme, path := w.getProbeSchemeAndPath(ctx, newSlice)
				if newSlice.Labels["probe"] == "true" {
					w.registry.UpdateFromEndpointSlice(newSlice, scheme, path)
				} else {
					w.registry.RemoveEndpointSlice(newSlice, scheme, path)
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			// 1. Try to cast the object directly to an EndpointSlice
			slice, ok := obj.(*discoveryv1.EndpointSlice)
			
			// 2. If it's not a direct slice, check if it's a tombstone (missed deletion event)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return // Not a slice and not a tombstone, safely ignore
				}
				
				// Extract the actual slice from the tombstone
				slice, ok = tombstone.Obj.(*discoveryv1.EndpointSlice)
				if !ok {
					return
				}
			}

			// 3. Now that we safely have the 'slice', we can read its labels and clean up
			if slice.Labels["probe"] == "true" {
				scheme, path := w.getProbeSchemeAndPath(ctx, slice)
				w.registry.RemoveEndpointSlice(slice, scheme, path)
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
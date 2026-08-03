package prober

import (
	"fmt"
	"sync"

	discoveryv1 "k8s.io/api/discovery/v1"
)

type Registry struct {
	mu      sync.RWMutex
	targets map[string]string
}

func NewRegistry() *Registry {
	return &Registry{
		targets: make(map[string]string),
	}
}

func (r *Registry) UpdateFromEndpointSlice(slice *discoveryv1.EndpointSlice) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Default HTTP port if no ports are explicitly listed in the slice
	portVal := int32(80)
	if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
		portVal = *slice.Ports[0].Port
	}

	for _, ep := range slice.Endpoints {
		for _, addr := range ep.Addresses {
			// Construct full target URL from endpoint IP and port
			targetURL := fmt.Sprintf("http://%s:%d/healthz", addr, portVal)
			r.targets[targetURL] = slice.Namespace
		}
	}
}

func (r *Registry) RemoveEndpointSlice(slice *discoveryv1.EndpointSlice) {
	r.mu.Lock()
	defer r.mu.Unlock()
	portVal := int32(80)
	if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
		portVal = *slice.Ports[0].Port
	}

	for _, ep := range slice.Endpoints {
		for _, addr := range ep.Addresses {
			targetURL := fmt.Sprintf("http://%s:%d/healthz", addr, portVal)
			delete(r.targets, targetURL)
		}
	}
}

func (r *Registry) GetTargets() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targets := make([]string, 0, len(r.targets))
	for t := range r.targets {
		targets = append(targets, t)
	}
	return targets
}

package prober

import (
	"fmt"
	"strings"
	"sync"

	discoveryv1 "k8s.io/api/discovery/v1"
)

// TargetEvent represents a change in the discovered targets.
type TargetEvent struct {
	Target  string
	IsAdded bool
}

type Registry struct {
	mu      sync.RWMutex
	targets map[string]string
	Events  chan TargetEvent
}

func NewRegistry() *Registry {
	return &Registry{
		targets: make(map[string]string),
		Events:  make(chan TargetEvent, 1000),
	}
}

func (r *Registry) UpdateFromEndpointSlice(slice *discoveryv1.EndpointSlice, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if path == "" {
		path = "/healthz"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	portVal := int32(80)
	if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
		portVal = *slice.Ports[0].Port
	}

	for _, ep := range slice.Endpoints {
		for _, addr := range ep.Addresses {
			targetURL := fmt.Sprintf("http://%s:%d%s", addr, portVal, path)

			if _, exists := r.targets[targetURL]; !exists {
				r.targets[targetURL] = slice.Namespace
				r.Events <- TargetEvent{Target: targetURL, IsAdded: true}
			}
		}
	}
}

func (r *Registry) RemoveEndpointSlice(slice *discoveryv1.EndpointSlice, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if path == "" {
		path = "/healthz"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	portVal := int32(80)
	if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
		portVal = *slice.Ports[0].Port
	}

	for _, ep := range slice.Endpoints {
		for _, addr := range ep.Addresses {
			targetURL := fmt.Sprintf("http://%s:%d%s", addr, portVal, path)

			if _, exists := r.targets[targetURL]; exists {
				delete(r.targets, targetURL)
				r.Events <- TargetEvent{Target: targetURL, IsAdded: false}
			}
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
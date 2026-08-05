package prober

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"

	discoveryv1 "k8s.io/api/discovery/v1"
)

// TargetEvent represents a change in the discovered targets.
type TargetEvent struct {
	Target  string
	IsAdded bool
}

// Registry maintains the active probing targets and handles distributed sharding.
type Registry struct {
	mu            sync.RWMutex
	targets       map[string]string
	Events        chan TargetEvent
	selfPodIP     string
	clusterPodIPs []string
}

// NewRegistry initializes a new thread-safe registry.
func NewRegistry() *Registry {
	return &Registry{
		targets:       make(map[string]string),
		Events:        make(chan TargetEvent, 1000),
		clusterPodIPs: make([]string, 0),
	}
}

// SetSelfIP initializes the pod's own identity used for hash comparisons.
func (r *Registry) SetSelfIP(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selfPodIP = ip
}

// UpdatePeers updates the active replica topology and triggers a rebalance.
// This is called whenever the HPA scales the prober pods up or down.
func (r *Registry) UpdatePeers(peers []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Sort IPs to guarantee an identical hash topology across all replicas
	sortedPeers := make([]string, len(peers))
	copy(sortedPeers, peers)
	sort.Strings(sortedPeers)
	
	r.clusterPodIPs = sortedPeers
	r.rebalanceTargetsLocked()
}

// ShouldProcessTarget uses Rendezvous Hashing (Highest Random Weight) 
// to determine if this specific pod is responsible for probing the target.
func (r *Registry) ShouldProcessTarget(target string) bool {
	// Fallback: process all targets if standalone or unconfigured
	if r.selfPodIP == "" || len(r.clusterPodIPs) == 0 {
		return true 
	}

	var highestHash uint64
	var selectedPod string

	// Calculate the hash for the target against every active pod IP.
	// The pod that yields the highest hash value becomes the owner of this target.
	for _, podIP := range r.clusterPodIPs {
		h := hashTargetAndPod(target, podIP)
		if h > highestHash {
			highestHash = h
			selectedPod = podIP
		}
	}

	// Return true only if this pod won the hashing election
	return selectedPod == r.selfPodIP
}

// hashTargetAndPod generates a fast 64-bit FNV hash from the combined target URL and Pod IP.
func hashTargetAndPod(target, podIP string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(target + ":" + podIP))
	return h.Sum64()
}

// rebalanceTargetsLocked re-evaluates all known targets against the new topology.
// It emits events to start or stop internal schedulers if ownership has changed.
func (r *Registry) rebalanceTargetsLocked() {
	for targetURL := range r.targets {
		shouldProcess := r.ShouldProcessTarget(targetURL)
		
		// Send events to start/stop local execution based on the new ownership
		if shouldProcess {
			r.Events <- TargetEvent{Target: targetURL, IsAdded: true}
		} else {
			r.Events <- TargetEvent{Target: targetURL, IsAdded: false}
		}
	}
}

// UpdateFromEndpointSlice adds newly discovered targets from Kubernetes EndpointSlices.
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
				// Only emit an 'Added' event if this pod is responsible for the target
				if r.ShouldProcessTarget(targetURL) {
					r.Events <- TargetEvent{Target: targetURL, IsAdded: true}
				}
			}
		}
	}
}

// RemoveEndpointSlice removes deleted targets and stops their local schedulers.
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

// GetTargets returns all targets currently owned by this specific pod instance.
func (r *Registry) GetTargets() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targets := make([]string, 0, len(r.targets))
	for t := range r.targets {
		if r.ShouldProcessTarget(t) {
			targets = append(targets, t)
		}
	}
	return targets
}
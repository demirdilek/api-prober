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
	mu            sync.RWMutex // Protects targets and peer topology against race conditions during concurrent reads and writes
	targets       map[string]string // Stores all known targets in the cluster
	Events        chan TargetEvent // Channel emitting add/remove events for locally assigned targets
	selfPodIP     string // The Pod-IP of the prober himself
	clusterPodIPs []string // List of all active Prober Pod-IPs in the Cluster
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

	// Sort IPs to guarantee an identical and determistic hash topology across all replicas
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
	h := fnv.New64a() // Initialize fast 64-bit FNV-1a non-cryptographic hasher
	_, _ = h.Write([]byte(target + ":" + podIP)) // Hash the combined string (ignoring in-memory write errors)
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
// It now accepts a dynamic 'scheme' parameter to support TCP, TLS, gRPC, etc.
func (r *Registry) UpdateFromEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Provide a fallback scheme just in case an empty string slips through.
	if scheme == "" {
		scheme = "http"
	}
	
	// Ensure HTTP paths always start with a slash, but ignore completely empty paths 
	// (which are valid for raw TCP/TLS connections).
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	portVal := int32(80)
	if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
		portVal = *slice.Ports[0].Port
	}

	for _, ep := range slice.Endpoints {
		for _, addr := range ep.Addresses {
			// Dynamically construct the target URL (e.g., "tcp://10.0.0.1:80" or "http://10.0.0.1:8080/healthz")
			targetURL := fmt.Sprintf("%s://%s:%d%s", scheme, addr, portVal, path)

			if _, exists := r.targets[targetURL]; !exists {
				r.targets[targetURL] = slice.Namespace
				// Emit an 'Added' event only if the Rendezvous Hashing algorithm assigns this target to this specific pod.
				if r.ShouldProcessTarget(targetURL) {
					r.Events <- TargetEvent{Target: targetURL, IsAdded: true}
				}
			}
		}
	}
}

// RemoveEndpointSlice removes deleted targets and stops their local schedulers.
// It uses the same dynamic scheme parameter to accurately reconstruct and find the target URL.
func (r *Registry) RemoveEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if scheme == "" {
		scheme = "http"
	}
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	portVal := int32(80)
	if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
		portVal = *slice.Ports[0].Port
	}

	for _, ep := range slice.Endpoints {
		for _, addr := range ep.Addresses {
			// Reconstruct the exact URL to safely delete it from the registry map.
			targetURL := fmt.Sprintf("%s://%s:%d%s", scheme, addr, portVal, path)

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

// GetPeers returns a copy of the current sorted peer IPs for testing or debugging
func (r *Registry) GetPeers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peersCopy := make([]string, len(r.clusterPodIPs))
	copy(peersCopy, r.clusterPodIPs)

	return peersCopy
}
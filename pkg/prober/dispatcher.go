package prober

import (
	"context"
	"net/url"
	"sync"
)

// ProbeFunc defines the function pointer signature for any protocol prober.
type ProbeFunc func(ctx context.Context, target string) ErrorCategory

// Dispatcher manages protocol handlers via function pointers
type Dispatcher struct {
	mu      sync.RWMutex
	probers map[string]ProbeFunc
}

// NewDispatcher initializes an empty dispatcher map
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		probers: make(map[string]ProbeFunc),
	}
}

// Register assigns a ProbeFunc function pointer to a specific scheme (e.g., "http", "https")
func (d *Dispatcher) Register(scheme string, fn ProbeFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.probers[scheme] = fn
}

// Execute resolves the scheme from the target URL and runs the corresponding ProbeFunc
func (d *Dispatcher) Execute(ctx context.Context, target string) ErrorCategory {
	parsedURL, err := url.Parse(target)
	if err != nil {
		return CategoryUnknown
	}

	d.mu.RLock()
	fn, exists := d.probers[parsedURL.Scheme]
	d.mu.RUnlock()

	if !exists {
		return CategoryUnknown
	}

	// Direct execution via function pointer
	return fn(ctx, target)
}

package prober

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestTCPProber_ProbeTCPTarget(t *testing.T) {
	// Start a local dummy TCP server to test successful connections
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start local tcp listener: %v", err)
	}
	defer listener.Close()

	// Accept and immediately close incoming connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	validTarget := "tcp://" + listener.Addr().String()
	// Use an arbitrary unassigned high port to simulate connection refused
	refusedTarget := "tcp://127.0.0.1:59842" 

	prober := NewTCPProber()

	tests := []struct {
		name     string
		target   string
		timeout  time.Duration
		expected ErrorCategory
	}{
		{
			name:     "Successful TCP connection",
			target:   validTarget,
			timeout:  2 * time.Second,
			expected: "",
		},
		{
			name:     "Connection refused",
			target:   refusedTarget,
			timeout:  2 * time.Second,
			expected: CategoryConnect,
		},
		{
			name:     "Invalid URL format",
			target:   "%%%invalid-target",
			timeout:  2 * time.Second,
			expected: CategoryUnknown,
		},
		{
			name:     "Connection timeout",
			// Use a non-routable IP (TEST-NET-1) to force a timeout, combined with a tiny context deadline
			target:   "tcp://198.51.100.1:80",
			timeout:  1 * time.Millisecond,
			expected: CategoryTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			got := prober.ProbeTCPTarget(ctx, tt.target)
			if got != tt.expected {
				t.Errorf("ProbeTCPTarget() = %v, want %v", got, tt.expected)
			}
		})
	}
}
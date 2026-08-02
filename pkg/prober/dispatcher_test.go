package prober

import (
	"context"
	"testing"
)

func TestDispatcher(t *testing.T) {
	dispatcher := NewDispatcher()

	// Mock probe function simulating a successful HTTP execution
	mockHTTP := func(ctx context.Context, target string) ErrorCategory {
		return "" // Success
	}

	// Mock probe function simulating a DNS failure
	mockDNSFail := func(ctx context.Context, target string) ErrorCategory {
		return CategoryDNS
	}

	// Register function pointers
	dispatcher.Register("http", mockHTTP)
	dispatcher.Register("dns", mockDNSFail)

	tests := []struct {
		name   string
		target string
		want   ErrorCategory
	}{
		{
			name:   "Registered HTTP Scheme",
			target: "http://example.com",
			want:   "",
		},
		{
			name:   "Registered DNS Scheme",
			target: "dns://example.com",
			want:   CategoryDNS,
		},
		{
			name:   "Unregistered Scheme (gRPC)",
			target: "grpc://example.com",
			want:   CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dispatcher.Execute(context.Background(), tt.target)
			if got != tt.want {
				t.Errorf("dispatcher.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

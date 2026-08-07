package prober

import (
	"context"
	"net"
	"testing"
)

func TestMapToCategory(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		expected   ErrorCategory
	}{
		{
			name:       "HTTP Error",
			err:        nil,
			statusCode: 500,
			expected:   CategoryHTTP,
		},
		{
			name:       "DNS Resolution Error",
			err:        &net.DNSError{},
			statusCode: 0,
			expected:   CategoryDNS,
		},
		{
			name:       "Timeout Error",
			err:        context.DeadlineExceeded,
			statusCode: 0,
			expected:   CategoryTimeout,
		},
		{
			name:       "Success Target",
			err:        nil,
			statusCode: 200,
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapToCategory(tt.err, tt.statusCode)
			if got != tt.expected {
				t.Errorf("MapToCategory() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestErrorCategory_Hint(t *testing.T) {
	if CategoryDNS.Hint() == "" {
		t.Errorf("expected non-empty hint for CategoryDNS")
	}
	if CategoryHTTP.Hint() == "" {
		t.Errorf("expected non-empty hint for CategoryHTTP")
	}
}


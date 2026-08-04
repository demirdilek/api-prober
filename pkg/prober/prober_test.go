package prober

import (
	"context"
	"net"
	"testing"
	"net/http"
	"net/http/httptest"
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

func TestProbeHTTPTarget_BodyDiscarding(t *testing.T) {
	// Mock HTTP server returning a payload
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok payload"))
	}))
	defer ts.Close()

	prober := NewHTTPProber(ts.Client())
	errCat := prober.ProbeHTTPTarget(context.Background(), ts.URL)

	if errCat != "" {
		t.Errorf("expected no error category for successful probe, got %v", errCat)
	}
}
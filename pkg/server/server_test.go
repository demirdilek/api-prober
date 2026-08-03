package server

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

// English comments as requested

func TestServer_EndpointsAndLifecycle(t *testing.T) {
	// Use an arbitrary high port for local test server binding
	addr := "127.0.0.1:18080"
	srv := New(addr)

	// Start server in background goroutine
	go srv.Start()

	// Wait briefly for server to bind and listen
	time.Sleep(50 * time.Millisecond)

	baseURL := "http://" + addr

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Healthz endpoint",
			path:           "/healthz",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "Readyz endpoint",
			path:           "/readyz",
			expectedStatus: http.StatusOK,
			expectedBody:   "READY",
		},
		{
			name:           "Metrics endpoint",
			path:           "/metrics",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "Pprof debug index",
			path:           "/debug/pprof/",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
	}

	client := &http.Client{Timeout: 2 * time.Second}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Get(baseURL + tt.path)
			if err != nil {
				t.Fatalf("failed to perform GET request to %s: %v", tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d for %s, got %d", tt.expectedStatus, tt.path, resp.StatusCode)
			}

			if tt.expectedBody != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				if string(body) != tt.expectedBody {
					t.Errorf("expected body %q, got %q", tt.expectedBody, string(body))
				}
			}
		})
	}

	// Test graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Errorf("expected clean server shutdown, got error: %v", err)
	}
}
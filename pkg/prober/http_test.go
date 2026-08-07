package prober

import (
	"context"
	"testing"
	"net/http"
	"net/http/httptest"
)


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
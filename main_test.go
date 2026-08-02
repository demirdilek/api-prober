package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/demirdilek/kube-prober/pkg/prober"
)

// English comments as preferred

func TestContains(t *testing.T) {
	slice := []string{"http://service-a", "http://service-b", "http://service-c"}

	if !contains(slice, "http://service-b") {
		t.Errorf("Expected slice to contain 'http://service-b'")
	}

	if contains(slice, "http://service-d") {
		t.Errorf("Did not expect slice to contain 'http://service-d'")
	}
}

func TestGetEnvAsInt(t *testing.T) {
	envKey := "TEST_WORKERS_COUNT"
	defaultVal := 50

	// Test default value when env is empty
	os.Unsetenv(envKey)
	if val := getEnvAsInt(envKey, defaultVal); val != defaultVal {
		t.Errorf("Expected default value %d, got %d", defaultVal, val)
	}

	// Test valid integer input
	os.Setenv(envKey, "100")
	if val := getEnvAsInt(envKey, defaultVal); val != 100 {
		t.Errorf("Expected 100, got %d", val)
	}

	// Test invalid integer fallback
	os.Setenv(envKey, "invalid_number")
	if val := getEnvAsInt(envKey, defaultVal); val != defaultVal {
		t.Errorf("Expected fallback to default value %d on invalid input, got %d", defaultVal, val)
	}

	os.Unsetenv(envKey)
}

func TestProbeTarget_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	httpProber := prober.NewHTTPProber(server.Client())
	dispatcher := prober.NewDispatcher()
	dispatcher.Register("http", httpProber.ProbeHTTPTarget)

	// Perform probe on successful endpoint
	probeTarget(ctx, server.URL, dispatcher)
}

func TestProbeTarget_Non2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	httpProber := prober.NewHTTPProber(server.Client())
	dispatcher := prober.NewDispatcher()
	dispatcher.Register("http", httpProber.ProbeHTTPTarget)

	// Perform probe on failing endpoint
	probeTarget(ctx, server.URL, dispatcher)
}

func TestTargetScheduler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan Job, 10)
	interval := 50 * time.Millisecond
	target := "http://test-target.svc.cluster.local"

	var wg sync.WaitGroup
	wg.Add(1)

	go targetScheduler(ctx, target, jobs, interval, &wg)

	// Verify immediate first execution
	select {
	case job := <-jobs:
		if job.Target != target {
			t.Errorf("Expected target %s, got %s", target, job.Target)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for initial scheduled job")
	}

	// Verify subsequent tick execution
	select {
	case job := <-jobs:
		if job.Target != target {
			t.Errorf("Expected target %s, got %s", target, job.Target)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for second scheduled job tick")
	}

	cancel()
	wg.Wait()
}

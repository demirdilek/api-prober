package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/demirdilek/kube-prober/pkg/prober"
	"github.com/demirdilek/kube-prober/pkg/server"
	"github.com/demirdilek/kube-prober/pkg/env"
	"github.com/demirdilek/kube-prober/pkg/kube"

)

func init() {
	prober.RegisterMetrics(prometheus.DefaultRegisterer)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Configure worker pool capacity and HTTP client options for heavy concurrent probing
	numWorkers := env.GetInt("WORKERS", 50)
	prober.MaxWorkersGauge.Set(float64(numWorkers))
	jobQueueSize := env.GetInt("QUEUE_SIZE", 10000)
	probeInterval := time.Duration(env.GetInt("PROBE_INTERVAL_SECONDS", 2)) * time.Second
	httpTimeout := time.Duration(env.GetInt("HTTP_TIMEOUT_SECONDS", 5)) * time.Second

	// Pre-configure HTTP transport with aggressive connection pooling for high-throughput reuse
	httpClient := &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        env.GetInt("MAX_IDLE_CONNS", 1000),
			MaxIdleConnsPerHost: env.GetInt("MAX_IDLE_CONNS_PER_HOST", 100),
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Register HTTP handlers
	httpProber := prober.NewHTTPProber(httpClient)
	dispatcher := prober.NewDispatcher()
	dispatcher.Register("http", httpProber.ProbeHTTPTarget)
	dispatcher.Register("https", httpProber.ProbeHTTPTarget)

	// Register TCP handler
	tcpProber := prober.NewTCPProber()
	dispatcher.Register("tcp", tcpProber.ProbeTCPTarget)

	// Setup graceful shutdown context listening for SIGINT and SIGTERM OS signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup
	jobs := make(chan prober.Job, jobQueueSize)

	// Spawn worker pool goroutines to process incoming probe jobs concurrently
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go prober.WorkerPool(ctx, jobs, dispatcher, &wg)
	}

	clientset := kube.InitClient()
	registry := prober.NewRegistry()
	
	// Retrieve local pod IP via Downward API for Rendezvous Hashing target ownership calculations
	selfIP := os.Getenv("POD_IP")
	if selfIP != "" {
		registry.SetSelfIP(selfIP)
	}

	// Initialize the unified KubeWatcher for both peer topology and target discovery
	watcher := prober.NewKubeWatcher(clientset, registry)

	// 1. Watch peer replicas dynamically to rebalance targets upon HPA scaling events
	go watcher.WatchPeers(ctx)

	// 2. Start the EndpointSlice informer in a background goroutine to stream target updates asynchronously
    go func() {
        if err := watcher.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
            slog.Error("Informer watcher stopped", "error", err)
        }
    }()

	activeSchedulers := make(map[string]context.CancelFunc)
	var schedMu sync.Mutex

	// Event loop: process target assignments emitted by the sharding registry
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-registry.Events:
				schedMu.Lock()
				
				if evt.IsAdded {
					// Target assigned to this replica: start local periodic probe scheduler
					if _, exists := activeSchedulers[evt.Target]; !exists {
						slog.Info("New target discovered", "target", evt.Target)
						schedCtx, schedCancel := context.WithCancel(ctx)
						activeSchedulers[evt.Target] = schedCancel
						
						wg.Add(1)
						go prober.TargetScheduler(schedCtx, evt.Target, jobs, probeInterval, &wg)
					}
					} else {
							// Target revoked or deleted: cancel local scheduler and purge metrics
							if cancelFunc, exists := activeSchedulers[evt.Target]; exists {
							slog.Info("Target removed", "target", evt.Target)
							cancelFunc()
							delete(activeSchedulers, evt.Target)
							prober.DeleteTargetMetrics(evt.Target) // <--- Clean up metrics here
						}
					}	
				schedMu.Unlock()
			}
		}
	}()

	// Start telemetry & health probe server (/metrics, /healthz, /readyz, /debug/pprof)
	srv := server.New(":8080")
	go srv.Start()

	<-ctx.Done()
	slog.Info("Shutting down cleanly...")

	// Allow up to 5 seconds for active HTTP probes and goroutines to finish gracefully
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_ = srv.Shutdown(shutdownCtx)
	wg.Wait()
	slog.Info("Goodbye.")
}

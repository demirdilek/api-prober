package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/demirdilek/kube-prober/pkg/prober"
	"github.com/demirdilek/kube-prober/pkg/server"
)

func init() {
	prober.RegisterMetrics(prometheus.DefaultRegisterer)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	numWorkers := getEnvAsInt("WORKERS", 50)
	prober.MaxWorkersGauge.Set(float64(numWorkers))
	jobQueueSize := getEnvAsInt("QUEUE_SIZE", 10000)
	probeInterval := time.Duration(getEnvAsInt("PROBE_INTERVAL_SECONDS", 2)) * time.Second
	httpTimeout := time.Duration(getEnvAsInt("HTTP_TIMEOUT_SECONDS", 5)) * time.Second

	httpClient := &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        getEnvAsInt("MAX_IDLE_CONNS", 1000),
			MaxIdleConnsPerHost: getEnvAsInt("MAX_IDLE_CONNS_PER_HOST", 100),
			IdleConnTimeout:     90 * time.Second,
		},
	}

	httpProber := prober.NewHTTPProber(httpClient)
	dispatcher := prober.NewDispatcher()
	dispatcher.Register("http", httpProber.ProbeHTTPTarget)
	dispatcher.Register("https", httpProber.ProbeHTTPTarget)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup
	jobs := make(chan prober.Job, jobQueueSize)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go prober.WorkerPool(ctx, jobs, dispatcher, &wg)
	}

	clientset := initKubeClient()
	registry := prober.NewRegistry()
	watcher := prober.NewTargetWatcher(clientset, registry)

	go func() {
		if err := watcher.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Informer watcher stopped", "error", err)
		}
	}()

	activeSchedulers := make(map[string]context.CancelFunc)
	var schedMu sync.Mutex

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				discoveredTargets := registry.GetTargets()
				schedMu.Lock()
				for target, cancelFunc := range activeSchedulers {
					if !prober.Contains(discoveredTargets, target) {
						slog.Info("Target removed", "target", target)
						cancelFunc()
						delete(activeSchedulers, target)
					}
				}
				for _, target := range discoveredTargets {
					if _, exists := activeSchedulers[target]; !exists {
						slog.Info("New target discovered", "target", target)
						schedCtx, schedCancel := context.WithCancel(ctx)
						activeSchedulers[target] = schedCancel
						wg.Add(1)
						go prober.TargetScheduler(schedCtx, target, jobs, probeInterval, &wg)
					}
				}
				schedMu.Unlock()
			}
		}
	}()

	srv := server.New(":8080")
	go srv.Start()

	<-ctx.Done()
	slog.Info("Shutting down cleanly...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_ = srv.Shutdown(shutdownCtx)
	wg.Wait()
	slog.Info("Goodbye.")
}

func initKubeClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			slog.Error("Failed to build k8s config", "error", err)
			os.Exit(1)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error("Failed to create k8s clientset", "error", err)
		os.Exit(1)
	}
	return clientset
}

func getEnvAsInt(name string, defaultVal int) int {
	valStr := os.Getenv(name)
	if valStr == "" {
		return defaultVal;
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal;
	}
	return val
}
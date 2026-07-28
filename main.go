package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	latencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "api_prober_latency_seconds",
			Help:    "The time taken to probe the target in seconds (Latency).",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"target"},
	)

	trafficCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_prober_traffic_total",
			Help: "Total number of probes sent to the target (Traffic).",
		},
		[]string{"target"},
	)

	errorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_prober_errors_total",
			Help: "Total number of failed probes (Errors).",
		},
		[]string{"target", "status_code"},
	)

	saturationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "api_prober_saturation_active_workers",
			Help: "Number of active concurrent probing workers (Saturation).",
		},
		[]string{"target"},
	)
)

func init() {
	prometheus.MustRegister(latencyHistogram)
	prometheus.MustRegister(trafficCounter)
	prometheus.MustRegister(errorCounter)
	prometheus.MustRegister(saturationGauge)
}

type Job struct {
	Target string
}

func probeTarget(ctx context.Context, target string, client *http.Client) {
	saturationGauge.WithLabelValues(target).Inc()
	defer saturationGauge.WithLabelValues(target).Dec()

	trafficCounter.WithLabelValues(target).Inc()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		slog.Error("Failed to create HTTP request", "target", target, "error", err)
		errorCounter.WithLabelValues(target, "request_creation_error").Inc()
		return
	}

	startTime := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(startTime).Seconds()

	if err != nil {
		slog.Warn("Target is unreachable or request timed out", "target", target, "error", err)
		errorCounter.WithLabelValues(target, "network_error").Inc()
		return
	}
	defer resp.Body.Close()

	latencyHistogram.WithLabelValues(target).Observe(duration)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusStr := strconv.Itoa(resp.StatusCode)
		errorCounter.WithLabelValues(target, statusStr).Inc()
		slog.Warn("Target returned non-2xx status code", "target", target, "status_code", resp.StatusCode)
	} else {
		slog.Debug("Target probed successfully", "target", target, "duration_seconds", duration)
	}
}

func workerPool(ctx context.Context, jobs <-chan Job, client *http.Client, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			probeTarget(ctx, job.Target, client)
		}
	}
}

func targetScheduler(ctx context.Context, target string, jobs chan<- Job, interval time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	select {
	case jobs <- Job{Target: target}:
	case <-ctx.Done():
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case jobs <- Job{Target: target}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// watchK8sServices implements Service Discovery using the K8s API
func watchK8sServices(ctx context.Context, jobs chan<- Job, interval time.Duration, activeSchedulers map[string]context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()

	// Try in-cluster config first, fallback to local kubeconfig for development
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			slog.Error("Failed to build k8s config", "error", err)
			return
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error("Failed to create k8s clientset", "error", err)
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// List services across all namespaces with label 'probe=true'
			services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
				LabelSelector: "probe=true",
			})
			if err != nil {
				slog.Error("Failed to list k8s services", "error", err)
				continue
			}

			var discoveredTargets []string
			for _, svc := range services.Items {
				probePath := "/status/200"
				if pathAnnot, ok := svc.Annotations["probe/path"]; ok && pathAnnot != "" {
					probePath = pathAnnot
				}

				for _, port := range svc.Spec.Ports {
					targetURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d%s", svc.Name, svc.Namespace, port.Port, probePath)
					discoveredTargets = append(discoveredTargets, targetURL)
				}
			}

			mu.Lock()
			for target, cancelFunc := range activeSchedulers {
				if !contains(discoveredTargets, target) {
					slog.Info("Target service removed, stopping scheduler", "target", target)
					cancelFunc()
					delete(activeSchedulers, target)
				}
			}

			for _, target := range discoveredTargets {
				if _, exists := activeSchedulers[target]; !exists {
					slog.Info("New K8s service discovered, allocating scheduler", "target", target)
					schedCtx, schedCancel := context.WithCancel(ctx)
					activeSchedulers[target] = schedCancel

					wg.Add(1)
					go targetScheduler(schedCtx, target, jobs, interval, wg)
				}
			}
			mu.Unlock()
		}
	}
}

func contains(slice []string, key string) bool {
	for _, item := range slice {
		if item == key {
			return true
		}
	}
	return false
}

func getEnvAsInt(name string, defaultVal int) int {
	valStr := os.Getenv(name)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	numWorkers := getEnvAsInt("WORKERS", 50)
	jobQueueSize := getEnvAsInt("QUEUE_SIZE", 10000)
	maxIdleConns := getEnvAsInt("MAX_IDLE_CONNS", 1000)
	maxIdleConnsPerHost := getEnvAsInt("MAX_IDLE_CONNS_PER_HOST", 100)
	probeIntervalSeconds := getEnvAsInt("PROBE_INTERVAL_SECONDS", 2)
	httpTimeoutSeconds := getEnvAsInt("HTTP_TIMEOUT_SECONDS", 5)

	probeInterval := time.Duration(probeIntervalSeconds) * time.Second

	client := &http.Client{
		Timeout: time.Duration(httpTimeoutSeconds) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        maxIdleConns,
			MaxIdleConnsPerHost: maxIdleConnsPerHost,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup
	activeSchedulers := make(map[string]context.CancelFunc)
	jobs := make(chan Job, jobQueueSize)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go workerPool(ctx, jobs, client, &wg)
	}

	wg.Add(1)
	go watchK8sServices(ctx, jobs, probeInterval, activeSchedulers, &wg)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	})

	srv := &http.Server{Addr: ":8080", Handler: mux}

	go func() {
		slog.Info("Metric server starting on :8080")
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server failed to run", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("Received shutdown signal, initiating graceful termination...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_ = srv.Shutdown(shutdownCtx)
	wg.Wait()
	slog.Info("api-prober stack components stopped cleanly. Goodbye.")
}

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
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
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
	
	// 1. Retrieve and set own Pod IP injected by the Kubernetes Downward API
	selfIP := os.Getenv("POD_IP")
	if selfIP != "" {
		registry.SetSelfIP(selfIP)
	}

	// 2. Start the peer watcher in the background to monitor HPA scaling events
	go watchProberPeers(ctx, clientset, registry)

	watcher := prober.NewTargetWatcher(clientset, registry)

	go func() {
		if err := watcher.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Informer watcher stopped", "error", err)
		}
	}()

	activeSchedulers := make(map[string]context.CancelFunc)
	var schedMu sync.Mutex

go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-registry.Events:
				schedMu.Lock()
				
				if evt.IsAdded {
					if _, exists := activeSchedulers[evt.Target]; !exists {
						slog.Info("New target discovered", "target", evt.Target)
						schedCtx, schedCancel := context.WithCancel(ctx)
						activeSchedulers[evt.Target] = schedCancel
						
						wg.Add(1)
						go prober.TargetScheduler(schedCtx, evt.Target, jobs, probeInterval, &wg)
					}
					} else {
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

// watchProberPeers observes the EndpointSlice of the kube-prober service itself.
// It dynamically updates the registry with the active replica topology whenever the HPA scales.
func watchProberPeers(ctx context.Context, clientset *kubernetes.Clientset, registry *prober.Registry) {
	// Create an Informer for EndpointSlices in the deployment's namespace
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		10*time.Minute,
		informers.WithNamespace("default"), // Adjust if deployed to a different namespace
	)

	informer := factory.Discovery().V1().EndpointSlices().Informer()

	updatePeers := func() {
		var peerIPs []string
		
		// Iterate over all discovered EndpointSlices in the cache
		for _, obj := range informer.GetStore().List() {
			slice, ok := obj.(*discoveryv1.EndpointSlice)
			
			// Filter specifically for the kube-prober service
			if ok && slice.Labels["kubernetes.io/service-name"] == "kube-prober" {
				for _, ep := range slice.Endpoints {
					// Only consider pods that are marked as Ready
					if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
						peerIPs = append(peerIPs, ep.Addresses...)
					}
				}
			}
		}
		
		// If we found active replicas, push the new topology to the registry for rebalancing
		if len(peerIPs) > 0 {
			registry.UpdatePeers(peerIPs)
		}
	}

	// Register event handlers for add/update/delete events
	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { updatePeers() },
		UpdateFunc: func(oldObj, newObj interface{}) { updatePeers() },
		DeleteFunc: func(obj interface{}) { updatePeers() },
	})

	factory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	
	// Perform an initial sync to populate the registry at startup
	updatePeers()
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
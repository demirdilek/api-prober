package prober

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Latency measures probing duration in seconds per target.
	LatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "kube_prober_latency_seconds",
			Help: "The time taken to probe the target in seconds (Latency).",
		},
		[]string{"target"},
	)

	// Traffic tracks the total number of probe requests sent per target.
	TrafficCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_prober_traffic_total",
			Help: "Total number of probes sent to the target (Traffic).",
		},
		[]string{"target"},
	)

	// Errors tracks failed probes categorized by target and error category.
	ErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_prober_errors_total",
			Help: "Total number of failed probes (Errors).",
		},
		[]string{"target", "category"},
	)

	// Saturation gauges current capacity by counting active concurrent worker goroutines.
	SaturationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kube_prober_saturation_active_workers",
			Help: "Number of active concurrent probing workers (Saturation).",
		},
		[]string{"target"},
	)

	// Max workers capacity metric (Saturation capacity)
	MaxWorkersGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_prober_saturation_max_workers",
			Help: "Maximum capacity of worker goroutines configured for the prober.",
		},
	)

	// CategoryHintInfo exposes troubleshooting hints per category as a static info metric.
	CategoryHintInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kube_prober_error_category_hint_info",
			Help: "Static mapping of error categories to actionable troubleshooting hints.",
		},
		[]string{"category", "hint"},
	)
)

// RegisterMetrics registers all prober metrics with the provided Prometheus registry.
func RegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		LatencyHistogram,
		TrafficCounter,
		ErrorCounter,
		SaturationGauge,
		MaxWorkersGauge,
		CategoryHintInfo,
	)

	// Populate static category hints directly from ErrorCategory.Hint()
	categories := []ErrorCategory{
		CategoryDNS,
		CategoryConnect,
		CategoryTLS,
		CategoryTimeout,
		CategoryHTTP,
		CategoryUnknown,
	}

	for _, cat := range categories {
		CategoryHintInfo.WithLabelValues(string(cat), cat.Hint()).Set(1)
	}
}

// DeleteTargetMetrics removes Prometheus metric entries for a deleted target to prevent memory leaks.
func DeleteTargetMetrics(target string) {
	LatencyHistogram.DeleteLabelValues(target)
	TrafficCounter.DeleteLabelValues(target)
	SaturationGauge.DeleteLabelValues(target)

	// Clean up all error categories for this target
	categories := []ErrorCategory{
		CategoryDNS,
		CategoryConnect,
		CategoryTLS,
		CategoryTimeout,
		CategoryHTTP,
		CategoryUnknown,
	}
	for _, cat := range categories {
		ErrorCounter.DeleteLabelValues(target, string(cat))
	}
}
package prober

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	prometheus_dto "github.com/prometheus/client_model/go"
)

func TestRegisterMetrics_Success(t *testing.T) {
	reg := prometheus.NewRegistry()

	// Direct call without prober.
	RegisterMetrics(reg)

	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	var hintFamily *prometheus_dto.MetricFamily
	for _, mf := range metricFamilies {
		if mf.GetName() == "kube_prober_error_category_hint_info" {
			hintFamily = mf
			break
		}
	}

	if hintFamily == nil {
		t.Fatal("expected metric kube_prober_error_category_hint_info to be registered")
	}

	expectedCategoriesCount := 6
	if len(hintFamily.GetMetric()) != expectedCategoriesCount {
		t.Errorf("expected %d hint metric entries, got %d", expectedCategoriesCount, len(hintFamily.GetMetric()))
	}
}

func TestDeleteTargetMetrics(t *testing.T) {
	target := "http://10.244.0.50:8080/healthz"

	// 1. Populate metrics with label values
	TrafficCounter.WithLabelValues(target).Inc()
	SaturationGauge.WithLabelValues(target).Set(1)
	LatencyHistogram.WithLabelValues(target).Observe(0.05)
	ErrorCounter.WithLabelValues(target, string(CategoryHTTP)).Inc()

	// 2. Delete target metrics
	DeleteTargetMetrics(target)

	// 3. Verify metrics no longer contain the target label series
	reg := prometheus.NewRegistry()
	RegisterMetrics(reg)

	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	for _, mf := range metricFamilies {
		for _, m := range mf.GetMetric() {
			for _, label := range m.GetLabel() {
				if label.GetName() == "target" && label.GetValue() == target {
					t.Errorf("expected metric %s label series for target %s to be deleted, but it still exists", mf.GetName(), target)
				}
			}
		}
	}
}
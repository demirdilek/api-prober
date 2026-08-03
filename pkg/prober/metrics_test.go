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
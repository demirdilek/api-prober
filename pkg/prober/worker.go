package prober

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	Target string
}

// ProbeTarget executes a probe against a target and records 4 Golden Signals metrics.
func ProbeTarget(ctx context.Context, target string, dispatcher *Dispatcher) {
	SaturationGauge.WithLabelValues(target).Inc()
	defer SaturationGauge.WithLabelValues(target).Dec()

	TrafficCounter.WithLabelValues(target).Inc()

	startTime := time.Now()
	errCat := dispatcher.Execute(ctx, target)
	duration := time.Since(startTime).Seconds()

	LatencyHistogram.WithLabelValues(target).Observe(duration)

	if errCat != "" {
		ErrorCounter.WithLabelValues(target, string(errCat)).Inc()
		slog.Warn("Target probing failed",
			"target", target,
			"error_category", errCat,
			"hint", errCat.Hint(),
		)
	} else {
		slog.Debug("Target probed successfully", "target", target, "duration_seconds", duration)
	}
}

// WorkerPool processes incoming probe jobs.
func WorkerPool(ctx context.Context, jobs <-chan Job, dispatcher *Dispatcher, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			ProbeTarget(ctx, job.Target, dispatcher)
		}
	}
}

// TargetScheduler pushes probe jobs into the jobs channel periodically.
func TargetScheduler(ctx context.Context, target string, jobs chan<- Job, interval time.Duration, wg *sync.WaitGroup) {
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

func Contains(slice []string, key string) bool {
	for _, item := range slice {
		if item == key {
			return true
		}
	}
	return false
}
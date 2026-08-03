package prober

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestContains(t *testing.T) {
	slice := []string{"http://service-a", "http://service-b"}

	if !Contains(slice, "http://service-a") {
		t.Errorf("expected slice to contain http://service-a")
	}

	if Contains(slice, "http://service-c") {
		t.Errorf("did not expect slice to contain http://service-c")
	}
}

func TestWorkerPool_Execution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	jobs := make(chan Job, 5)
	dispatcher := NewDispatcher()

	dispatcher.Register("http", func(ctx context.Context, target string) ErrorCategory {
		return ""
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go WorkerPool(ctx, jobs, dispatcher, &wg)

	jobs <- Job{Target: "http://example.com"}
	close(jobs)

	wg.Wait()
}

func TestTargetScheduler_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	jobs := make(chan Job, 10)

	var wg sync.WaitGroup
	wg.Add(1)

	go TargetScheduler(ctx, "http://test-target", jobs, 10*time.Millisecond, &wg)

	time.Sleep(25 * time.Millisecond)
	cancel()

	wg.Wait()
	close(jobs)

	count := 0
	for range jobs {
		count++
	}

	if count == 0 {
		t.Errorf("expected scheduler to produce at least 1 job before cancellation")
	}
}
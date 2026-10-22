package lessons

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Table-Driven Tests for WorkerPool
// ---------------------------------------------------------------------------

func TestWorkerPool(t *testing.T) {
	tests := []struct {
		name        string
		numJobs     int
		numWorkers  int
		expectedLen int
	}{
		{
			name:        "Single job with single worker",
			numJobs:     1,
			numWorkers:  1,
			expectedLen: 1,
		},
		{
			name:        "Multiple jobs with multiple workers",
			numJobs:     10,
			numWorkers:  3,
			expectedLen: 10,
		},
		{
			name:        "More workers than jobs",
			numJobs:     5,
			numWorkers:  20,
			expectedLen: 5,
		},
		{
			name:        "High throughput batch",
			numJobs:     50,
			numWorkers:  8,
			expectedLen: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := WorkerPool(tt.numJobs, tt.numWorkers)

			if len(results) != tt.expectedLen {
				t.Fatalf("expected %d results, got %d", tt.expectedLen, len(results))
			}

			// Sort results because workers finish in non-deterministic order
			sort.Ints(results)

			// Verify squares 1^2, 2^2, ... N^2
			for i := 0; i < tt.numJobs; i++ {
				expectedSquare := (i + 1) * (i + 1)
				if results[i] != expectedSquare {
					t.Errorf("at index %d: expected %d, got %d", i, expectedSquare, results[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test for Channel Rendezvous & Deadlock Safety
// ---------------------------------------------------------------------------

func TestChannelMechanicsHandoff(t *testing.T) {
	ch := make(chan string)

	go func() {
		ch <- "ping"
	}()

	select {
	case msg := <-ch:
		if msg != "ping" {
			t.Errorf("expected 'ping', got %q", msg)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for unbuffered channel handoff")
	}
}

// ---------------------------------------------------------------------------
// Test for Context Cancellation
// ---------------------------------------------------------------------------

func TestContextCancellationPropagation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	doneCh := make(chan struct{})

	go func(c context.Context) {
		<-c.Done()
		close(doneCh)
	}(ctx)

	cancel()

	select {
	case <-doneCh:
		// Clean cancellation observed
	case <-time.After(500 * time.Millisecond):
		t.Fatal("goroutine did not observe context cancellation in time")
	}
}

// ---------------------------------------------------------------------------
// Race Detector Verification Test
// ---------------------------------------------------------------------------

func TestConcurrentGoroutineCounters(t *testing.T) {
	// Tests atomic / mutex protected shared states to ensure zero data races
	var mu sync.Mutex
	counter := 0
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			mu.Lock()
			counter++
			mu.Unlock()
		}()
	}

	wg.Wait()

	if counter != goroutines {
		t.Fatalf("expected counter to be %d, got %d", goroutines, counter)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks (run with go test -bench=. -benchmem)
// ---------------------------------------------------------------------------

func BenchmarkWorkerPool(b *testing.B) {
	benchmarks := []struct {
		name       string
		numJobs    int
		numWorkers int
	}{
		{"Jobs10_Workers2", 10, 2},
		{"Jobs50_Workers4", 50, 4},
		{"Jobs100_Workers8", 100, 8},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = WorkerPool(bm.numJobs, bm.numWorkers)
			}
		})
	}
}

func BenchmarkPrimeCalculation(b *testing.B) {
	b.Run("Sequential_50", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for j := 0; j < 50; j++ {
				_ = cpuWork()
			}
		}
	})

	b.Run("Concurrent_50", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(50)
			for j := 0; j < 50; j++ {
				go func() {
					defer wg.Done()
					_ = cpuWork()
				}()
			}
			wg.Wait()
		}
	})
}

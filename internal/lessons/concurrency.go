// Package lessons provides executable, heavily-commented Go lessons.
// This file covers Go concurrency fundamentals: goroutines, channels,
// worker pools, select multiplexing, context cancellation, and benchmarking.
package lessons

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// LESSON: GOROUTINES & SYNC.WAITGROUP
// =============================================================================
//
// Goroutines are lightweight threads managed by the Go runtime (M:N scheduler).
// They start with a tiny 2KB stack that dynamically grows/shrinks as needed.
// This allows a Go program to run hundreds of thousands of concurrent goroutines
// without exhausting OS thread or memory limits.
//
// sync.WaitGroup coordinates goroutines by maintaining an internal atomic counter:
//   - wg.Add(1): Increment counter before spawning the goroutine
//   - wg.Done(): Decrement counter when the goroutine finishes (use defer!)
//   - wg.Wait(): Block until counter reaches 0
//
// CRITICAL RULE: Always call wg.Add() in the PARENT goroutine before launching
// the child goroutine. If called inside the child, wg.Wait() might execute
// and return before the child ever gets scheduled to call wg.Add()!

// BasicGoroutines demonstrates spawning goroutines, sync.WaitGroup coordination,
// and loop variable capture mechanics.
func BasicGoroutines() {
	fmt.Println("━━━ Goroutines & sync.WaitGroup ━━━")

	var wg sync.WaitGroup

	// --- 1. Spawning multiple goroutines safely ---
	fmt.Println("\n  Spawning 3 worker goroutines:")
	for i := 1; i <= 3; i++ {
		// CRITICAL: wg.Add MUST be called before "go func()", not inside it.
		wg.Add(1)
		go func(id int) {
			defer wg.Done() // Ensure wg.Done() runs even if panic occurs
			fmt.Printf("    [Goroutine %d] Working...\n", id)
			time.Sleep(10 * time.Millisecond)
			fmt.Printf("    [Goroutine %d] Done!\n", id)
		}(i)
	}

	// Block until all 3 goroutines complete
	wg.Wait()
	fmt.Println("  All workers finished via wg.Wait()!")

	// --- 2. The Classic Loop Variable Capture Trap ---
	// In Go versions prior to Go 1.22, loop variables `i` were shared across
	// iterations. Goroutines that closed over `i` without passing it as an argument
	// would frequently print the final loop value for all iterations!
	// In Go 1.22+, loop variables have per-iteration scope by default.
	// But passing values as arguments remains best practice for readability and clarity.
	fmt.Println("\n  💡 Loop variable capture note:")
	fmt.Println("     In Go 1.22+, loop variables have per-iteration scope.")
	fmt.Println("     Still, passing the loop variable as an explicit argument")
	fmt.Println("     makes data ownership crystal clear to any reader.")
}

// =============================================================================
// LESSON: CHANNEL MECHANICS (Buffered vs. Unbuffered)
// =============================================================================
//
// "Do not communicate by sharing memory; instead, share memory by communicating."
// Channels are typed conduits through which you can send and receive values
// with the channel operator (<-).
//
// 1. UNBUFFERED CHANNEL (make(chan T)):
//    - Capacity = 0.
//    - SYNCHRONOUS HANDOFF: Send blocks until a receiver is ready;
//      receive blocks until a sender is ready.
//    - Guaranteed delivery and rendezvous point.
//
// 2. BUFFERED CHANNEL (make(chan T, capacity)):
//    - Capacity > 0.
//    - ASYNCHRONOUS up to buffer limit: Send only blocks when the buffer is FULL;
//      receive only blocks when the buffer is EMPTY.
//    - Decouples producer and consumer throughput.
//
// 3. CHANNEL OWNERSHIP PRINCIPLE:
//    - The sender/producer SHOULD CLOSE the channel when done.
//    - The receiver should NEVER close a channel (sending on closed channel panics!).
//    - Closing signals to receivers: `for val := range ch` will terminate cleanly.

// ChannelMechanics demonstrates unbuffered synchronous handoffs and buffered queues.
func ChannelMechanics() {
	fmt.Println("\n━━━ Channel Mechanics ━━━")

	// --- 1. Unbuffered Channel (Synchronous Rendezvous) ---
	fmt.Println("\n  1. Unbuffered channel handoff:")
	unbuf := make(chan string)

	go func() {
		fmt.Println("    [Producer] Preparing payload...")
		time.Sleep(20 * time.Millisecond)
		fmt.Println("    [Producer] Sending to unbuffered channel (blocking until consumed)...")
		unbuf <- "Secret Message"
		fmt.Println("    [Producer] Send complete! Receiver took it.")
	}()

	msg := <-unbuf
	fmt.Printf("    [Consumer] Received: %q\n", msg)

	// --- 2. Buffered Channel (Asynchronous Queue) ---
	fmt.Println("\n  2. Buffered channel (capacity = 3):")
	buf := make(chan int, 3)

	// Non-blocking sends until buffer is full
	buf <- 100
	buf <- 200
	buf <- 300
	fmt.Printf("    Buffer len: %d, cap: %d\n", len(buf), cap(buf))

	// Close the channel to indicate no more items will be sent
	close(buf)

	// Read all items. Ranging over a closed channel drains the buffer and exits!
	fmt.Print("    Draining buffer: ")
	for val := range buf {
		fmt.Printf("%d ", val)
	}
	fmt.Println()

	// Reading from an empty, closed channel immediately returns zero value & false
	val, ok := <-buf
	fmt.Printf("    Reading from closed empty channel -> val: %d, ok: %t (safe, no panic!)\n", val, ok)
}

// =============================================================================
// LESSON: WORKER POOL PATTERN (Bounded Concurrency)
// =============================================================================
//
// Spawning an unbounded number of goroutines (e.g., 100,000 for 100,000 tasks)
// can exhaust memory, open file descriptors, or database connections.
//
// The Worker Pool pattern bounds concurrency to a fixed number of workers (N).
// N workers read from a single `jobs` channel and write to a `results` channel.
// This provides:
//   - Predictable resource utilization
//   - Built-in load balancing (faster workers pull more jobs)
//   - Graceful backpressure

// WorkerPool runs numJobs across numWorkers using the fan-out/fan-in pattern.
func WorkerPool(numJobs, numWorkers int) []int {
	jobs := make(chan int, numJobs)
	results := make(chan int, numJobs)

	var wg sync.WaitGroup

	// Spawn N workers (Fan-out)
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := range jobs {
				// Simulate unit of work (e.g., square the number)
				time.Sleep(5 * time.Millisecond)
				results <- j * j
			}
		}(w)
	}

	// Send all jobs to the jobs channel
	for j := 1; j <= numJobs; j++ {
		jobs <- j
	}
	close(jobs) // Close jobs channel so workers exit their `for range` loops

	// Wait for all workers to finish, then close results channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results (Fan-in)
	var output []int
	for res := range results {
		output = append(output, res)
	}

	return output
}

// WorkerPoolDemo prints a demonstration of the worker pool.
func WorkerPoolDemo() {
	fmt.Println("\n━━━ Worker Pool Pattern ━━━")
	jobsCount := 6
	workersCount := 3
	fmt.Printf("  Processing %d jobs with %d concurrent workers...\n", jobsCount, workersCount)

	start := time.Now()
	res := WorkerPool(jobsCount, workersCount)
	elapsed := time.Since(start)

	fmt.Printf("  Results: %v\n", res)
	fmt.Printf("  Completed in %v (notice work was distributed in parallel!)\n", elapsed)
}

// =============================================================================
// LESSON: SELECT MULTIPLEXING & TIMEOUTS
// =============================================================================
//
// The `select` statement lets a goroutine wait on multiple channel operations.
// It blocks until one of its cases is ready to proceed.
// If multiple are ready, Go chooses one at random (preventing starvation).
//
// Key patterns:
//   - `time.After(duration)`: Implement request timeouts
//   - `default:`: Non-blocking channel read/write

// SelectMultiplexing demonstrates multiplexing between channels and handling timeouts.
func SelectMultiplexing() {
	fmt.Println("\n━━━ Select Multiplexing & Timeouts ━━━")

	fastCh := make(chan string)
	slowCh := make(chan string)

	go func() {
		time.Sleep(10 * time.Millisecond)
		fastCh <- "Fast response"
	}()

	go func() {
		time.Sleep(50 * time.Millisecond)
		slowCh <- "Slow response"
	}()

	// Multiplexing between two channels
	for i := 0; i < 2; i++ {
		select {
		case msg := <-fastCh:
			fmt.Printf("  Received from fastCh: %s\n", msg)
		case msg := <-slowCh:
			fmt.Printf("  Received from slowCh: %s\n", msg)
		}
	}

	// Timeout Pattern with time.After
	fmt.Println("\n  Timeout demonstration:")
	stalledCh := make(chan string)
	go func() {
		time.Sleep(200 * time.Millisecond) // Will take longer than timeout
		stalledCh <- "Too late!"
	}()

	select {
	case res := <-stalledCh:
		fmt.Printf("  Received: %s\n", res)
	case <-time.After(30 * time.Millisecond):
		fmt.Println("  ⏰ Operation timed out after 30ms! (Protected against deadlock)")
	}
}

// =============================================================================
// LESSON: CONTEXT CANCELLATION & GRACEFUL PROPAGATION
// =============================================================================
//
// In production Go services, cancellation and deadlines must propagate
// across API boundaries and goroutine trees.
//
// The `context.Context` interface provides:
//   - `ctx.Done()`: channel closed when the context is cancelled or times out
//   - `ctx.Err()`: error explaining why the context was cancelled
//   - `ctx.Deadline()`: when the context will be cancelled
//
// NEVER ignore `ctx.Done()` in long-running goroutines, or you will cause
// GOROUTINE LEAKS!

// ContextCancellation demonstrates context-driven shutdown.
func ContextCancellation() {
	fmt.Println("\n━━━ Context Cancellation ━━━")

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)

	go func(ctx context.Context) {
		defer wg.Done()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Context was cancelled! Clean up and exit cleanly.
				fmt.Printf("  [Goroutine] Received ctx.Done(): %v. Exiting cleanly!\n", ctx.Err())
				return
			case t := <-ticker.C:
				fmt.Printf("  [Goroutine] Working at %s...\n", t.Format("15:04:05.000"))
			}
		}
	}(ctx)

	// Let the worker run for a brief moment
	time.Sleep(65 * time.Millisecond)

	fmt.Println("  [Main] Cancelling context now...")
	cancel() // Signals ctx.Done()

	wg.Wait()
	fmt.Println("  [Main] Worker stopped cleanly. No goroutine leaks!")
}

// =============================================================================
// BENCHMARK: SEQUENTIAL VS. CONCURRENT EXECUTION
// =============================================================================
//
// Demonstrates the raw throughput power of Go concurrency for parallelizable
// workloads, returning the durations for the web dashboard.

// isPrime is a CPU-bound helper function for benchmark tasks.
func isPrime(n int) bool {
	if n <= 1 {
		return false
	}
	for i := 2; i*i <= n; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}

// cpuWork simulates a deterministic CPU-bound task (counting primes up to 15,000).
func cpuWork() int {
	count := 0
	for i := 2; i <= 15000; i++ {
		if isPrime(i) {
			count++
		}
	}
	return count
}

// BenchmarkSequentialVsConcurrent runs a CPU-bound task N times sequentially,
// then N times concurrently using goroutines and WaitGroup.
// It returns (seqDuration, concDuration).
func BenchmarkSequentialVsConcurrent(n int) (seqDuration, concDuration time.Duration) {
	if n <= 0 {
		n = 100
	}

	// 1. Sequential Run
	startSeq := time.Now()
	for i := 0; i < n; i++ {
		_ = cpuWork()
	}
	seqDuration = time.Since(startSeq)

	// 2. Concurrent Run using Goroutines + sync.WaitGroup
	startConc := time.Now()
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = cpuWork()
		}()
	}
	wg.Wait()
	concDuration = time.Since(startConc)

	return seqDuration, concDuration
}

// RunConcurrencyLesson runs all concurrency lessons and prints benchmark results.
func RunConcurrencyLesson() {
	RunAllConcurrencyLessons()
}

// RunAllConcurrencyLessons executes all concurrency-related lessons in sequence.
func RunAllConcurrencyLessons() {
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║     📚 LESSON: Go Concurrency Mechanics      ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	BasicGoroutines()
	ChannelMechanics()
	WorkerPoolDemo()
	SelectMultiplexing()
	ContextCancellation()

	fmt.Println("\n━━━ Quick Benchmark (10 iterations) ━━━")
	seqDur, concDur := BenchmarkSequentialVsConcurrent(10)
	fmt.Printf("  Sequential Duration: %v\n", seqDur)
	fmt.Printf("  Concurrent Duration: %v\n", concDur)
	if concDur > 0 {
		fmt.Printf("  ⚡ Speedup: %.2fx faster!\n", float64(seqDur)/float64(concDur))
	}
}

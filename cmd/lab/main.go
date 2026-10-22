// Package main is the entrypoint for the Go Mastery Lab — an interactive
// CLI + HTTP server that exposes Go lessons, benchmarks, runtime metrics,
// and a sharded KV-store playground through a single-page web dashboard.
//
// Architecture overview:
//   - The HTML dashboard is embedded at compile-time via Go's embed package.
//   - Each lesson is executed by temporarily redirecting os.Stdout to a
//     pipe so that the lesson's fmt.Print* output is captured as a string.
//   - Graceful shutdown is implemented by listening for OS signals
//     (SIGINT / SIGTERM) and draining in-flight HTTP requests before exit.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gomastery/internal/apps/kvstore"
	"gomastery/internal/lessons"
	"gomastery/web"
)

// ---------------------------------------------------------------------------
// Static assets
// ---------------------------------------------------------------------------
// The dashboard assets are embedded via the gomastery/web package.

// ---------------------------------------------------------------------------
// captureOutput — stdout redirection helper
// ---------------------------------------------------------------------------
// captureOutput executes fn while os.Stdout is temporarily replaced with the
// write end of an OS pipe. Everything fn prints via fmt.Print* (which writes
// to os.Stdout) is collected from the read end.
//
// WHY os.Pipe instead of bytes.Buffer?
// fmt.Println and friends write to the file descriptor behind os.Stdout.
// A bytes.Buffer is an in-memory io.Writer that has no file descriptor, so
// assigning it to os.Stdout would cause a panic.  os.Pipe gives us a real
// *os.File that we can swap in safely.
//
// Thread-safety note: we use a mutex to serialise calls so two concurrent
// lesson requests do not interleave each other's stdout hijacking.
var stdoutMu sync.Mutex

func captureOutput(fn func()) string {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	// 1. Create an OS-level pipe (reader ↔ writer).
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Sprintf("pipe error: %v", err)
	}

	// 2. Save the real stdout so we can restore it later.
	origStdout := os.Stdout
	os.Stdout = w // fn's fmt.Print* calls will now go into the pipe.

	// 3. Spin up a goroutine that reads everything from the pipe.
	//    We must read in a separate goroutine because the pipe has a
	//    finite buffer; if fn writes more than the buffer size without
	//    a concurrent reader the write blocks and we deadlock.
	outCh := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		outCh <- string(data)
	}()

	// 4. Execute the caller-supplied function.
	fn()

	// 5. Close the write end so the reader goroutine sees EOF.
	w.Close()

	// 6. Restore the original stdout immediately.
	os.Stdout = origStdout

	// 7. Wait for the reader goroutine to finish and return the output.
	output := <-outCh
	r.Close()

	return output
}

// ---------------------------------------------------------------------------
// HTTP handler functions
// ---------------------------------------------------------------------------

// handleLesson dispatches to the appropriate lesson by name, captures the
// printed output, and returns it as plain text.
func handleLesson(w http.ResponseWriter, r *http.Request) {
	// Extract the lesson name from the URL path.
	// Expected format: /api/lesson/{name}
	// We split the path and grab the last segment.
	parts := strings.Split(strings.TrimRight(r.URL.Path, "/"), "/")
	if len(parts) == 0 {
		http.Error(w, "missing lesson name", http.StatusBadRequest)
		return
	}
	name := parts[len(parts)-1]

	var output string

	switch strings.ToLower(name) {
	case "memory":
		output = captureOutput(func() { lessons.RunMemoryLesson() })
	case "interfaces":
		output = captureOutput(func() { lessons.RunInterfacesLesson() })
	case "concurrency":
		output = captureOutput(func() { lessons.RunConcurrencyLesson() })
	default:
		http.Error(w, fmt.Sprintf("unknown lesson: %q", name), http.StatusNotFound)
		return
	}

	// Return captured text as plain text.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(output))
}

// benchmarkResult is the JSON envelope returned by the benchmark endpoint.
type benchmarkResult struct {
	SequentialMs float64 `json:"sequential_ms"`
	ConcurrentMs float64 `json:"concurrent_ms"`
	Speedup      float64 `json:"speedup"`
}

// handleBenchmark runs the sequential-vs-concurrent benchmark and returns
// timing results as JSON.
func handleBenchmark(w http.ResponseWriter, r *http.Request) {
	// Parse the "n" query parameter (number of work items).
	// Default to 100 if missing or invalid.
	nStr := r.URL.Query().Get("n")
	n, err := strconv.Atoi(nStr)
	if err != nil || n <= 0 {
		n = 100
	}

	// Run the benchmark.  The function returns durations for both modes.
	seqDur, concDur := lessons.BenchmarkSequentialVsConcurrent(n)

	// Compute the speedup ratio.  Guard against division by zero (the
	// concurrent duration could theoretically be zero for tiny n).
	speedup := 0.0
	if concDur > 0 {
		speedup = float64(seqDur) / float64(concDur)
	}

	result := benchmarkResult{
		SequentialMs: float64(seqDur.Milliseconds()),
		ConcurrentMs: float64(concDur.Milliseconds()),
		Speedup:      speedup,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// metricsResponse is the JSON envelope for runtime memory statistics.
type metricsResponse struct {
	Goroutines  int     `json:"goroutines"`
	HeapAllocMB float64 `json:"heap_alloc_mb"`
	HeapSysMB   float64 `json:"heap_sys_mb"`
	NumGC       uint32  `json:"num_gc"`
	HeapInUseMB float64 `json:"heap_in_use_mb"`
}

// handleMetrics reads the Go runtime's memory statistics and returns them
// as JSON.  This is called every 2 seconds by the dashboard's auto-refresh.
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Convert byte counts to megabytes for readability.
	const mb = 1024.0 * 1024.0

	resp := metricsResponse{
		Goroutines:  runtime.NumGoroutine(),
		HeapAllocMB: float64(m.HeapAlloc) / mb,
		HeapSysMB:   float64(m.HeapSys) / mb,
		NumGC:       m.NumGC,
		HeapInUseMB: float64(m.HeapInuse) / mb,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------------------
// KV Store HTTP handlers
// ---------------------------------------------------------------------------

// kvSetRequest is the expected JSON body for the SET endpoint.
type kvSetRequest struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// handleKVSet handles POST /api/kv/set.
// It deserialises the JSON body and stores the key with an optional TTL.
func handleKVSet(store *kvstore.ShardedMap) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req kvSetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// Compute TTL duration — zero means no expiration.
		ttl := time.Duration(req.TTLSeconds) * time.Second
		store.Set(req.Key, req.Value, ttl)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"key":    req.Key,
		})
	}
}

// handleKVGet handles GET /api/kv/get?key=...
func handleKVGet(store *kvstore.ShardedMap) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing key parameter"})
			return
		}

		value, found := store.Get(key)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key":   key,
			"value": value,
			"found": found,
		})
	}
}

// handleKVDelete handles DELETE /api/kv/delete?key=...
func handleKVDelete(store *kvstore.ShardedMap) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := r.URL.Query().Get("key")
		if key == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing key parameter"})
			return
		}

		store.Delete(key)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "deleted",
			"key":    key,
		})
	}
}

// ---------------------------------------------------------------------------
// Route setup
// ---------------------------------------------------------------------------

// setupRoutes registers all HTTP handlers on the default mux.
func setupRoutes(mux *http.ServeMux, store *kvstore.ShardedMap) {
	// --- Static files (embedded dashboard) --------------------------------
	// web.Files serves dashboard.html directly at the root of the embedded filesystem.
	fileServer := http.FileServer(http.FS(web.Files))

	// Serve the root path — redirect to the dashboard.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If the request is exactly "/" serve dashboard.html directly.
		if r.URL.Path == "/" {
			r.URL.Path = "/dashboard.html"
		}
		fileServer.ServeHTTP(w, r)
	})

	// --- API routes -------------------------------------------------------
	mux.HandleFunc("/api/lesson/", handleLesson)
	mux.HandleFunc("/api/benchmark", handleBenchmark)
	mux.HandleFunc("/api/metrics", handleMetrics)

	mux.Handle("/api/kv/set", handleKVSet(store))
	mux.Handle("/api/kv/get", handleKVGet(store))
	mux.Handle("/api/kv/delete", handleKVDelete(store))
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	// --- Parse command-line flags ------------------------------------------
	// We do a simple manual parse instead of importing flag so the import
	// list matches the spec exactly.  Look for "-port <value>" in os.Args.
	port := 8080
	for i, arg := range os.Args {
		if arg == "-port" && i+1 < len(os.Args) {
			if p, err := strconv.Atoi(os.Args[i+1]); err == nil && p > 0 {
				port = p
			}
		}
	}

	// --- Initialise the sharded KV store ----------------------------------
	store := kvstore.NewShardedMap(32) // 32 shards — a good default

	// --- Build HTTP server ------------------------------------------------
	mux := http.NewServeMux()
	setupRoutes(mux, store)

	addr := fmt.Sprintf(":%d", port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// --- Startup banner ---------------------------------------------------
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║      🚀  Go Mastery Lab — Server Started       ║")
	fmt.Printf("║      🌐  http://localhost:%d                  ║\n", port)
	fmt.Println("║      Press Ctrl+C to shut down gracefully       ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")

	// --- Graceful shutdown setup ------------------------------------------
	// We create a channel that receives OS signals.  When SIGINT or SIGTERM
	// arrives (e.g. Ctrl+C) we initiate a graceful shutdown with a 5-second
	// deadline so in-flight requests can complete.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start listening in a goroutine so the main goroutine can block on
	// the signal channel.
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Block until we receive a shutdown signal.
	sig := <-sigCh
	fmt.Printf("\n⚠  Received signal %v — shutting down…\n", sig)

	// Create a context with a 5-second deadline for the shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown gracefully: stop accepting new connections and wait for
	// in-flight requests to finish (up to the deadline).
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	// Close the KV store (stop its background expiry goroutine, etc.).
	store.Close()

	fmt.Println("✅  Server stopped cleanly. Goodbye!")
}

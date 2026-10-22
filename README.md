# 🚀 GoMastery: Interactive Lab & High-Throughput Benchmark Suite

> A hands-on, zero-dependency Go learning lab and concurrency testbed built entirely with the **Go Standard Library** (Go 1.22+ / 1.27+).

![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?style=flat&logo=go)
![Tests](https://img.shields.io/badge/Tests-100%25%20Passing%20(Race%20Safe)-success)
![Stdlib](https://img.shields.io/badge/Dependencies-Standard%20Library%20Only-blue)

---

## 📌 Project Overview

This repository is designed as a daily practice ground to master Go runtime internals, memory layouts, and high-concurrency patterns without third-party frameworks.

### What's Inside:

* 🧠 **Interactive Lessons (`internal/lessons/`)**:
  * `memory.go`: Slice header mechanics (`ptr`, `len`, `cap`), the silent 3-element reslice trap, 3-index slice syntax (`s[low:high:max]`), value vs. pointer semantics, and escape analysis (`-gcflags="-m"`).
  * `interfaces.go`: Implicit interface satisfaction (duck typing), composition via struct embedding, generics (`Filter[T]`, `Map[T, U]`), and modern error wrapping chains (`fmt.Errorf %w`, `errors.Is`, `errors.As`).
  * `concurrency.go`: Goroutines, buffered vs unbuffered channels, worker pool fan-out/fan-in, `select` timeout multiplexing, `context.Context` cancellation propagation, and CPU benchmark comparison.
* 🛠️ **Portfolio Applications (`internal/apps/`)**:
  * `kvstore/`: High-throughput, 32-shard concurrent in-memory Key-Value store with `sync.RWMutex` per shard, FNV-1a hashing, lazy TTL eviction on `Get`, and active background eviction ticker with leak-free termination.
  * `crawler/`: Concurrent site auditor using counting semaphore channels (`chan struct{}`), rate limiting (`time.Ticker`), `sync.Map` URL deduplication, and context-aware graceful shutdown.
* 📊 **Single-Page Embedded Dashboard (`web/` & `cmd/lab/`)**:
  * Pure CSS/JS dark dashboard embedded at compile time via `embed.FS`.
  * Real-time `runtime.MemStats` monitoring (Goroutine counts, heap usage, GC runs).
  * Pipe-based stdout redirection (`os.Pipe`) allowing you to trigger lessons and stream terminal output live into your browser.
  * Interactive Key-Value store playground.
* 🥊 **Pedagogical Masterclass Challenges**:
  * Check [`ARCHITECTURE_AND_EXERCISES.md`](./ARCHITECTURE_AND_EXERCISES.md) for 5 hands-on "Break & Fix" challenges (Zombie Goroutines, Slice Overwrites, ABBA Lock Ordering Deadlocks, WaitGroup Inversion, and Unbuffered Channel Leaks).

---

## 🏃 Quick Start

### 1. Run the Interactive Server & Dashboard
```bash
go run ./cmd/lab
```
Open **[http://localhost:8080](http://localhost:8080)** in your browser.

### 2. Run All Tests with Race Detector
```bash
go test -v -race ./...
```
*(Guarantees zero data races across 100 concurrent goroutine workloads).*

### 3. Run Microbenchmarks & Memory Allocation Profiles
```bash
go test -bench=. -benchmem ./...
```

---

## 🔬 Benchmark Highlights (Apple Silicon M5)

| Target Benchmark | Operations | ns/op | Memory (B/op) | Allocs/op | Speedup / Throughput |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `kvstore.BenchmarkGet` | 43,011,973 | **27.50 ns/op** | 23 B/op | 1 alloc/op | ~36.3M ops/sec |
| `kvstore.BenchmarkSet` | 16,869,589 | **68.83 ns/op** | 53 B/op | 3 allocs/op | ~14.5M ops/sec |
| `kvstore.BenchmarkConcurrentReadWrite` | 20,768,611 | **52.38 ns/op** | 25 B/op | 2 allocs/op | Parallel Scaling |
| `lessons.Sequential_50` | 186 | **6,098,939 ns/op** | 0 B/op | 0 allocs/op | 1x Baseline |
| `lessons.Concurrent_50` | 972 | **1,241,302 ns/op** | 836 B/op | 51 allocs/op | **~4.91x Speedup** |

---

## 📂 Repository Structure

```text
├── ARCHITECTURE_AND_EXERCISES.md    # Guided tour & 5 break-and-fix challenges
├── cmd/
│   └── lab/
│       └── main.go                  # CLI & HTTP server entrypoint
├── internal/
│   ├── apps/
│   │   ├── crawler/
│   │   │   └── crawler.go           # Portfolio App 1: Concurrent site auditor
│   │   └── kvstore/
│   │       ├── kvstore.go           # Portfolio App 2: 32-shard concurrent KV store
│   │       └── kvstore_test.go      # Table tests, race tests & benchmarks
│   └── lessons/
│       ├── memory.go                # Slice headers, pointer semantics, escape analysis
│       ├── interfaces.go            # Implicit satisfaction, generics, error trees
│       ├── concurrency.go           # Goroutines, channels, worker pools, context
│       └── concurrency_test.go      # Concurrency tests & alloc benchmarks
├── web/
│   ├── dashboard.html               # Embedded single-page dashboard
│   └── web.go                       # embed.FS packaging
├── go.mod                           # Go module definition
└── .gitignore                       # Clean Git configuration
```

---

## 📅 Daily Learning Plan

- [x] **Day 1**: Slice header internals, 3-index slicing, and escape analysis (`memory.go`)
- [x] **Day 2**: Structural typing, composition over inheritance, generics, error wrapping (`interfaces.go`)
- [x] **Day 3**: Goroutines, channel rendezvous, worker pools, `select` timeouts (`concurrency.go`)
- [x] **Day 4**: Sharded KV store, FNV-1a hashing, `sync.RWMutex`, TTL eviction (`kvstore.go`)
- [x] **Day 5**: Counting semaphores, rate limiting, and graceful `SIGINT` draining (`crawler.go`)
- [ ] **Day 6**: Break & Fix Challenges 1 & 2 (`ARCHITECTURE_AND_EXERCISES.md`)
- [ ] **Day 7**: Break & Fix Challenges 3, 4 & 5 (`ARCHITECTURE_AND_EXERCISES.md`)

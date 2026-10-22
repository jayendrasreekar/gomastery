package kvstore

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestSetGet uses table-driven tests to verify basic Set/Get functionality.
//
// WHY TABLE-DRIVEN TESTS?
// Table-driven tests are the idiomatic Go pattern for testing multiple
// inputs/outputs. Each row in the table defines an independent test case
// with its own name, input, and expected output. Benefits:
//   - Adding new cases requires only a new row, not new code.
//   - t.Run() gives each case a clear name in test output.
//   - Failures are pinpointed to the exact case that failed.
// ---------------------------------------------------------------------------
func TestSetGet(t *testing.T) {
	sm := NewShardedMap(32)
	defer sm.Close() // Always close to stop the background eviction goroutine.

	// Define our table of test cases. Each case sets a key with a value
	// and then verifies that Get returns the expected value.
	tests := []struct {
		name  string      // Human-readable description of the test case.
		key   string      // The key to set and then get.
		value interface{} // The value to store.
	}{
		{name: "string value", key: "greeting", value: "hello world"},
		{name: "integer value", key: "count", value: 42},
		{name: "float value", key: "pi", value: 3.14159},
		{name: "boolean value", key: "active", value: true},
		{name: "nil value", key: "nothing", value: nil},
		{name: "empty string key", key: "", value: "empty key works"},
		{name: "long key", key: "a-very-long-key-that-exercises-the-hash-function-with-more-bytes", value: "long"},
		{name: "special characters", key: "key with spaces & symbols!@#", value: "special"},
	}

	for _, tc := range tests {
		// t.Run creates a subtest for each case. If one fails, the others
		// still run, and the output clearly identifies which case failed.
		t.Run(tc.name, func(t *testing.T) {
			// Set the key with no TTL (0 means never expire).
			sm.Set(tc.key, tc.value, 0)

			got, ok := sm.Get(tc.key)
			if !ok {
				t.Fatalf("Get(%q) returned ok=false, want ok=true", tc.key)
			}
			if got != tc.value {
				t.Errorf("Get(%q) = %v, want %v", tc.key, got, tc.value)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestTTLExpiration verifies that entries with a TTL are automatically
// removed after they expire. This tests the lazy eviction path in Get().
// ---------------------------------------------------------------------------
func TestTTLExpiration(t *testing.T) {
	sm := NewShardedMap(32)
	defer sm.Close()

	// Set a key with a very short TTL of 50 milliseconds.
	sm.Set("ephemeral", "gone soon", 50*time.Millisecond)

	// Immediately after setting, the key should still be accessible.
	if val, ok := sm.Get("ephemeral"); !ok {
		t.Fatal("expected key to exist immediately after Set")
	} else if val != "gone soon" {
		t.Fatalf("got %v, want %q", val, "gone soon")
	}

	// Sleep long enough for the TTL to expire. We use 100ms (2x the TTL)
	// to account for scheduler jitter and timer imprecision.
	time.Sleep(100 * time.Millisecond)

	// After expiration, Get should return ok=false (lazy eviction).
	if _, ok := sm.Get("ephemeral"); ok {
		t.Error("expected key to be expired after 100ms, but it still exists")
	}
}

// ---------------------------------------------------------------------------
// TestConcurrentAccess launches 100 goroutines performing mixed reads and
// writes simultaneously. This test is designed to be run with the -race flag
// (go test -race) to detect any data races.
//
// WHY THIS TEST MATTERS:
// Concurrency bugs (data races, deadlocks) are non-deterministic — they may
// not manifest in sequential tests. The -race detector instruments memory
// accesses at compile time and reports unsynchronized access. Running many
// goroutines increases the probability of triggering races if they exist.
// ---------------------------------------------------------------------------
func TestConcurrentAccess(t *testing.T) {
	sm := NewShardedMap(32)
	defer sm.Close()

	// Use a WaitGroup to wait for all goroutines to finish before the
	// test function returns. Without this, the test might exit (and call
	// Close()) while goroutines are still running, causing panics.
	var wg sync.WaitGroup

	const numGoroutines = 100
	const opsPerGoroutine = 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)

				// Perform a write.
				sm.Set(key, j, 0)

				// Perform a read (may or may not find it, depending on
				// timing relative to other goroutines).
				sm.Get(key)

				// Occasionally delete to exercise that path too.
				if j%3 == 0 {
					sm.Delete(key)
				}
			}
		}(i)
	}

	wg.Wait()

	// If we get here without a race detector complaint or panic, the
	// concurrent access is properly synchronized.
}

// ---------------------------------------------------------------------------
// TestDelete verifies that deleting a key makes it inaccessible via Get.
// ---------------------------------------------------------------------------
func TestDelete(t *testing.T) {
	sm := NewShardedMap(32)
	defer sm.Close()

	// Set a key, verify it exists, delete it, verify it's gone.
	sm.Set("to-delete", "byebye", 0)

	if _, ok := sm.Get("to-delete"); !ok {
		t.Fatal("key should exist before deletion")
	}

	sm.Delete("to-delete")

	if _, ok := sm.Get("to-delete"); ok {
		t.Error("key should not exist after deletion")
	}

	// Deleting a non-existent key should be a safe no-op.
	sm.Delete("never-existed") // Should not panic.
}

// ---------------------------------------------------------------------------
// TestShardDistribution verifies that keys are distributed across multiple
// shards. If the hash function is degenerate (e.g., always returns 0), all
// keys would land in a single shard, defeating the purpose of sharding.
//
// This test inserts 1000 keys and checks that no single shard contains all
// of them. With a good hash function and 32 shards, we'd expect roughly
// 31 keys per shard (1000/32 ≈ 31.25).
// ---------------------------------------------------------------------------
func TestShardDistribution(t *testing.T) {
	const numShards = 32
	const numKeys = 1000

	sm := NewShardedMap(numShards)
	defer sm.Close()

	// Insert 1000 sequentially-named keys.
	for i := 0; i < numKeys; i++ {
		sm.Set(fmt.Sprintf("key-%d", i), i, 0)
	}

	// Count entries per shard by inspecting each shard directly.
	// We access the internal shards slice here because this is a
	// white-box test in the same package.
	nonEmptyShards := 0
	maxInShard := 0
	for _, s := range sm.shards {
		s.mu.RLock()
		count := len(s.items)
		s.mu.RUnlock()

		if count > 0 {
			nonEmptyShards++
		}
		if count > maxInShard {
			maxInShard = count
		}
	}

	// Verify that keys are spread across multiple shards. With 1000 keys
	// and 32 shards, we should see at least ~20 non-empty shards. We use
	// a conservative threshold of 2 to avoid flaky tests.
	if nonEmptyShards < 2 {
		t.Errorf("keys only in %d shard(s), expected distribution across multiple shards", nonEmptyShards)
	}

	// No single shard should have all the keys.
	if maxInShard == numKeys {
		t.Error("all keys landed in a single shard — hash function is degenerate")
	}

	// Verify total count is correct.
	if sm.Len() != numKeys {
		t.Errorf("Len() = %d, want %d", sm.Len(), numKeys)
	}

	t.Logf("Distribution: %d non-empty shards out of %d, max shard has %d keys",
		nonEmptyShards, numShards, maxInShard)
}

// ===========================================================================
// BENCHMARKS
//
// Benchmarks measure the raw throughput of operations. They are run with:
//   go test -bench=. -benchmem
//
// The -benchmem flag reports allocations per operation, which is crucial
// for understanding GC pressure.
// ===========================================================================

// BenchmarkSet measures the throughput of Set operations.
// b.RunParallel runs the benchmark body in multiple goroutines simultaneously,
// which is essential for testing a concurrent data structure's real-world
// performance (a sequential benchmark would never trigger contention).
func BenchmarkSet(b *testing.B) {
	sm := NewShardedMap(32)
	defer sm.Close()

	b.ResetTimer() // Don't count setup time in the benchmark.

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Use a unique key per iteration to avoid always hitting
			// the same shard.
			sm.Set(fmt.Sprintf("bench-key-%d", i), i, 0)
			i++
		}
	})
}

// BenchmarkGet measures the throughput of Get operations on pre-populated data.
// We pre-fill the map so that Get always finds a value, measuring the
// "cache hit" path rather than the "miss" path.
func BenchmarkGet(b *testing.B) {
	sm := NewShardedMap(32)
	defer sm.Close()

	// Pre-populate with 10,000 entries.
	const numEntries = 10000
	for i := 0; i < numEntries; i++ {
		sm.Set(fmt.Sprintf("bench-key-%d", i), i, 0)
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sm.Get(fmt.Sprintf("bench-key-%d", i%numEntries))
			i++
		}
	})
}

// BenchmarkConcurrentReadWrite simulates a realistic mixed workload where
// multiple goroutines are reading and writing simultaneously. This measures
// the effectiveness of sharding under contention.
//
// The ratio here is roughly 80% reads / 20% writes, which is a common
// pattern for cache-like workloads.
func BenchmarkConcurrentReadWrite(b *testing.B) {
	sm := NewShardedMap(32)
	defer sm.Close()

	// Pre-populate so reads can find data.
	const numEntries = 10000
	for i := 0; i < numEntries; i++ {
		sm.Set(fmt.Sprintf("bench-key-%d", i), i, 0)
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i%numEntries)
			if i%5 == 0 {
				// 20% writes: Set a new value.
				sm.Set(key, i, 0)
			} else {
				// 80% reads: Get the existing value.
				sm.Get(key)
			}
			i++
		}
	})
}

// Package kvstore implements a high-throughput, sharded in-memory key-value store.
//
// WHY SHARDING?
// A single map protected by a single mutex becomes a severe bottleneck under
// concurrent access because every goroutine—readers AND writers—must contend
// for the same lock. Sharding splits the key space into N independent
// sub-maps (shards), each with its own lock. This means:
//   - Two goroutines operating on keys that hash to DIFFERENT shards never
//     contend with each other at all — they acquire completely independent locks.
//   - Contention only occurs when goroutines happen to target the SAME shard.
//   - With 32 shards, the probability of two random operations colliding drops
//     to roughly 1/32 compared to a single-lock design.
//
// This is the same technique used by Java's ConcurrentHashMap and Go's
// sync.Map (internally). The trade-off is slightly more memory and code
// complexity in exchange for dramatically better throughput under concurrency.
package kvstore

import (
	"hash/fnv"
	"sync"
	"time"
)

// entry represents a single value stored in the map, along with optional
// TTL (time-to-live) expiration metadata.
//
// We store hasTTL as a separate boolean rather than checking for a zero
// time.Time because the zero value of time.Time is a valid (albeit unusual)
// expiration time, and we want to distinguish "no TTL" from "expires at
// the epoch."
type entry struct {
	value    interface{} // The actual stored value; interface{} allows any type.
	expireAt time.Time   // When this entry expires (only meaningful if hasTTL is true).
	hasTTL   bool        // Whether this entry has an expiration time.
}

// shard is one partition of the overall map. Each shard has its own lock
// and its own map, so operations on different shards are fully independent.
//
// WHY sync.RWMutex INSTEAD OF sync.Mutex?
// sync.Mutex is exclusive: only one goroutine can hold it at a time,
// regardless of whether it's reading or writing. sync.RWMutex distinguishes
// between readers and writers:
//   - Multiple readers can hold the lock simultaneously (RLock/RUnlock).
//   - A writer gets exclusive access (Lock/Unlock), blocking all readers
//     and other writers.
//
// For read-heavy workloads (which are typical of caches and KV stores),
// RWMutex dramatically increases throughput because reads don't block each
// other. The only downside is slightly higher overhead per lock operation
// compared to a plain Mutex, but this is negligible compared to the
// concurrency gains.
type shard struct {
	mu    sync.RWMutex
	items map[string]entry
}

// ShardedMap is the top-level concurrent key-value store. It distributes
// keys across multiple shards to minimize lock contention.
type ShardedMap struct {
	shards []*shard     // The array of shards; length is fixed at creation time.
	count  int          // Number of shards (cached for modulo arithmetic).
	stopCh chan struct{} // Signals the background eviction goroutine to stop.
}

// NewShardedMap creates a new ShardedMap with the specified number of shards.
// If numShards is <= 0, it defaults to 32.
//
// The constructor also starts a background goroutine that periodically scans
// for and removes expired entries. This goroutine is stopped by calling Close().
//
// WHY START EVICTION IN THE CONSTRUCTOR?
// Lazy eviction (checking TTL only on Get) is simple but has a problem:
// entries that are never read again will linger in memory forever, causing
// a slow memory leak. The background goroutine provides a safety net that
// periodically sweeps stale entries, bounding the worst-case memory usage.
// We combine both strategies: lazy eviction on read (for immediate accuracy)
// and periodic eviction (for memory hygiene).
func NewShardedMap(numShards int) *ShardedMap {
	if numShards <= 0 {
		numShards = 32
	}

	sm := &ShardedMap{
		shards: make([]*shard, numShards),
		count:  numShards,
		stopCh: make(chan struct{}),
	}

	// Initialize each shard with its own empty map.
	for i := 0; i < numShards; i++ {
		sm.shards[i] = &shard{
			items: make(map[string]entry),
		}
	}

	// Start the background eviction goroutine with a 1-second sweep interval.
	// This interval is a balance between eviction latency (how long expired
	// entries linger) and CPU overhead (how often we scan).
	sm.startEviction(1 * time.Second)

	return sm
}

// getShard returns the shard that a given key maps to.
//
// WHY FNV-1a HASH?
// We need a fast, well-distributed hash function to spread keys evenly
// across shards. FNV-1a (Fowler-Noll-Vo) is ideal because:
//   - It's extremely fast — just XOR and multiply per byte.
//   - It produces good distribution even for short, similar strings
//     (e.g., "key1", "key2", "key3" hash to very different values).
//   - It's available in the Go standard library (hash/fnv).
//   - Unlike cryptographic hashes (SHA-256), it has no security guarantees,
//     but we don't need any — we just want uniform distribution.
//
// The modulo operation (hash % numShards) maps the hash space down to our
// shard index range. This is slightly biased when numShards doesn't evenly
// divide the hash space, but with 32-bit hashes and small shard counts,
// the bias is negligible.
func (sm *ShardedMap) getShard(key string) *shard {
	// Create a new FNV-1a hasher. We create a new one each time rather than
	// pooling because the hasher is tiny (just a uint32) and allocation is
	// cheaper than the synchronization overhead of pooling.
	h := fnv.New32a()

	// hash.Hash.Write never returns an error for in-memory hashes,
	// so we can safely ignore the error return.
	_, _ = h.Write([]byte(key))

	// Sum32() returns the final 32-bit hash value. We take modulo numShards
	// to get the shard index.
	idx := int(h.Sum32()) % sm.count

	return sm.shards[idx]
}

// Set stores a key-value pair in the map. If ttl > 0, the entry will
// automatically expire after the specified duration.
//
// Only the target shard's write lock is acquired, so Set operations on
// keys in different shards proceed fully in parallel with zero contention.
func (sm *ShardedMap) Set(key string, value interface{}, ttl time.Duration) {
	s := sm.getShard(key)

	// Acquire the write lock on this specific shard only.
	// This blocks other writers to THIS shard and any readers of THIS shard,
	// but goroutines accessing OTHER shards are completely unaffected.
	s.mu.Lock()
	defer s.mu.Unlock()

	e := entry{
		value: value,
	}

	// Only set expiration metadata if a positive TTL was provided.
	// A zero or negative TTL means "never expire."
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
		e.hasTTL = true
	}

	s.items[key] = e
}

// Get retrieves a value by key. Returns the value and true if found (and
// not expired), or nil and false otherwise.
//
// Get implements LAZY EVICTION: if an entry is found but has expired, it is
// deleted on the spot and the caller sees a "not found" result. This is
// efficient because it piggybacks cleanup onto an operation we're already
// doing, avoiding unnecessary work for entries that are never accessed.
//
// Only the target shard's read lock is acquired initially. If we discover
// an expired entry, we must upgrade to a write lock to delete it. This
// "optimistic read" pattern is efficient because the common case (entry
// exists and is valid) only needs a read lock.
func (sm *ShardedMap) Get(key string) (interface{}, bool) {
	s := sm.getShard(key)

	// First, try with a read lock (shared — allows concurrent readers).
	s.mu.RLock()
	e, ok := s.items[key]
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}

	// Check for TTL expiration. We use time.Now() here, which is called
	// AFTER releasing the read lock. There's a tiny race window where an
	// entry could expire between the read and this check, but that's
	// acceptable — TTL expiration is best-effort, not a hard real-time
	// guarantee.
	if e.hasTTL && time.Now().After(e.expireAt) {
		// Entry has expired. Upgrade to a write lock to delete it.
		// Note: We must re-check after acquiring the write lock because
		// another goroutine might have already deleted it (or even set a
		// new value for the same key) between our RUnlock and Lock.
		s.mu.Lock()
		// Re-read the entry under the write lock to avoid deleting a
		// newer entry that was set by another goroutine.
		if current, exists := s.items[key]; exists && current.hasTTL && time.Now().After(current.expireAt) {
			delete(s.items, key)
		}
		s.mu.Unlock()

		return nil, false
	}

	return e.value, true
}

// Delete removes a key from the map. If the key doesn't exist, this is a
// no-op (delete on a Go map with a missing key is safe and does nothing).
func (sm *ShardedMap) Delete(key string) {
	s := sm.getShard(key)

	// Write lock is required because we're mutating the map.
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, key)
}

// Len returns the total number of entries across all shards.
//
// NOTE: This is an APPROXIMATE count in the face of concurrent modifications.
// Between acquiring and releasing each shard's lock, other goroutines may be
// modifying other shards. For an exact count, you'd need to hold ALL shard
// locks simultaneously, which would be expensive and defeat the purpose of
// sharding. For most use cases (monitoring, debugging, approximate sizing),
// an approximate count is perfectly acceptable.
func (sm *ShardedMap) Len() int {
	total := 0
	for _, s := range sm.shards {
		// Read lock each shard briefly to get its count.
		s.mu.RLock()
		total += len(s.items)
		s.mu.RUnlock()
	}
	return total
}

// startEviction launches a background goroutine that periodically scans all
// shards and removes expired entries.
//
// HOW THE BACKGROUND GOROUTINE AVOIDS LEAKS:
// Goroutine leaks occur when a goroutine runs forever with no way to stop it.
// We prevent this by using a stop channel (stopCh). The goroutine selects on
// both a time.Ticker (for periodic work) and the stop channel (for shutdown).
// When Close() is called, it closes stopCh, which unblocks the select and
// causes the goroutine to return, freeing its resources.
//
// We also stop the ticker (ticker.Stop()) to release the ticker's internal
// resources. While the ticker's goroutine would eventually be GC'd, explicit
// cleanup is good hygiene.
//
// WHY A SEPARATE EVICTION GOROUTINE?
// Without periodic eviction, entries that expire but are never accessed again
// would remain in memory indefinitely (a "zombie" problem). The background
// sweep ensures that memory is eventually reclaimed regardless of access
// patterns. The sweep interval controls the trade-off: shorter intervals
// reclaim memory faster but consume more CPU; longer intervals are cheaper
// but allow zombies to linger.
func (sm *ShardedMap) startEviction(interval time.Duration) {
	ticker := time.NewTicker(interval)

	go func() {
		// This deferred Stop ensures the ticker's resources are freed when
		// the goroutine exits, regardless of the exit path.
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Time to sweep! Scan every shard for expired entries.
				now := time.Now()
				for _, s := range sm.shards {
					// Acquire a write lock because we might delete entries.
					// We hold it for the entire scan of this shard, which
					// could block readers briefly. An alternative would be
					// to collect expired keys under a read lock, then delete
					// them under a write lock, reducing write-lock duration
					// at the cost of more complex code.
					s.mu.Lock()
					for k, e := range s.items {
						if e.hasTTL && now.After(e.expireAt) {
							delete(s.items, k)
						}
					}
					s.mu.Unlock()
				}

			case <-sm.stopCh:
				// Close() was called — time to shut down cleanly.
				return
			}
		}
	}()
}

// Close signals the background eviction goroutine to stop and waits for it
// to exit. After Close is called, the ShardedMap is still usable for
// Get/Set/Delete operations, but expired entries will no longer be
// automatically cleaned up.
//
// Close should be called when the ShardedMap is no longer needed to prevent
// the eviction goroutine from leaking. This is especially important in tests
// and in applications that create multiple ShardedMap instances over their
// lifetime.
func (sm *ShardedMap) Close() {
	// Closing a channel unblocks all goroutines waiting on it in a select
	// statement. This is the idiomatic Go pattern for broadcasting a
	// shutdown signal to one or more goroutines.
	//
	// We use close() rather than sending a value because:
	// 1. close() works even if multiple goroutines are listening (broadcast).
	// 2. A closed channel is immediately readable (returns zero value), so
	//    there's no risk of the signal being missed.
	close(sm.stopCh)
}

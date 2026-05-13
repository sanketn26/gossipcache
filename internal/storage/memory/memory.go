package memory

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sanketn26/gossipcache/internal/storage"
)

// MemoryStorage implements in-memory cache storage
// SRP: Responsible only for storing and retrieving data
type MemoryStorage struct {
	data     *shardedMap
	stats    *storageStats
	maxSize  int64
	eviction EvictionPolicy
	closed   atomic.Bool
	closeCh  chan struct{}
	wg       sync.WaitGroup
}

type storageStats struct {
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

// New creates a new MemoryStorage
func New(maxSize int64, evictionPolicy string) (*MemoryStorage, error) {
	eviction, err := newEvictionPolicy(evictionPolicy)
	if err != nil {
		return nil, err
	}

	ms := &MemoryStorage{
		data:     newShardedMap(256),
		stats:    &storageStats{},
		maxSize:  maxSize,
		eviction: eviction,
		closeCh:  make(chan struct{}),
	}

	// Start expiration goroutine
	ms.wg.Add(1)
	go ms.expirationLoop()

	return ms, nil
}

func (ms *MemoryStorage) Get(ctx context.Context, key string) (*storage.Entry, error) {
	if ms.closed.Load() {
		return nil, storage.ErrClosed
	}

	entry, ok := ms.data.get(key)
	if !ok {
		ms.stats.misses.Add(1)
		return nil, storage.ErrKeyNotFound
	}

	// Check expiration
	if entry.IsExpired() {
		ms.data.delete(key)
		ms.stats.misses.Add(1)
		return nil, storage.ErrKeyNotFound
	}

	ms.stats.hits.Add(1)
	ms.eviction.OnAccess(key)

	return cloneEntry(entry), nil
}

func (ms *MemoryStorage) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ms.closed.Load() {
		return storage.ErrClosed
	}

	now := time.Now()
	entry := &storage.Entry{
		Key:       key,
		Value:     append([]byte(nil), value...),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if ttl > 0 {
		entry.ExpiresAt = now.Add(ttl)
	}

	ms.data.set(key, entry)
	ms.eviction.OnAdd(key)

	for ms.shouldEvict() {
		if err := ms.evict(); err != nil {
			return err
		}
	}

	return nil
}

func (ms *MemoryStorage) Delete(ctx context.Context, key string) error {
	if ms.closed.Load() {
		return storage.ErrClosed
	}

	ms.data.delete(key)
	ms.eviction.OnRemove(key)

	return nil
}

func (ms *MemoryStorage) GetMulti(ctx context.Context, keys []string) (map[string]*storage.Entry, error) {
	result := make(map[string]*storage.Entry)

	for _, key := range keys {
		entry, err := ms.Get(ctx, key)
		if err == nil {
			result[key] = entry
		}
	}

	return result, nil
}

func (ms *MemoryStorage) SetMulti(ctx context.Context, entries map[string]*storage.Entry) error {
	for key, entry := range entries {
		ttl := time.Until(entry.ExpiresAt)
		if err := ms.Set(ctx, key, entry.Value, ttl); err != nil {
			return err
		}
	}
	return nil
}

func (ms *MemoryStorage) Keys(ctx context.Context) ([]string, error) {
	if ms.closed.Load() {
		return nil, storage.ErrClosed
	}

	return ms.data.keys(), nil
}

func (ms *MemoryStorage) Stats(ctx context.Context) (*storage.Stats, error) {
	return &storage.Stats{
		Keys:      int64(ms.data.len()),
		Size:      ms.currentSize(),
		Hits:      ms.stats.hits.Load(),
		Misses:    ms.stats.misses.Load(),
		Evictions: ms.stats.evictions.Load(),
	}, nil
}

func (ms *MemoryStorage) Close() error {
	if ms.closed.Swap(true) {
		return nil // Already closed
	}

	close(ms.closeCh)
	ms.wg.Wait()

	return nil
}

func (ms *MemoryStorage) shouldEvict() bool {
	return ms.currentSize() > ms.maxSize
}

func (ms *MemoryStorage) currentSize() int64 {
	// Approximate size calculation
	keys := ms.data.keys()
	var size int64

	for _, key := range keys {
		entry, ok := ms.data.get(key)
		if ok {
			size += int64(len(key) + len(entry.Value))
		}
	}

	return size
}

func (ms *MemoryStorage) evict() error {
	victim := ms.eviction.SelectVictim()
	if victim == "" {
		return fmt.Errorf("no victim to evict")
	}

	ms.data.delete(victim)
	ms.eviction.OnRemove(victim)
	ms.stats.evictions.Add(1)

	return nil
}

func (ms *MemoryStorage) expirationLoop() {
	defer ms.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ms.closeCh:
			return
		case <-ticker.C:
			ms.removeExpired()
		}
	}
}

func (ms *MemoryStorage) removeExpired() {
	keys := ms.data.keys()

	for _, key := range keys {
		entry, ok := ms.data.get(key)
		if ok && entry.IsExpired() {
			ms.data.delete(key)
			ms.eviction.OnRemove(key)
		}
	}
}

func cloneEntry(entry *storage.Entry) *storage.Entry {
	if entry == nil {
		return nil
	}

	clone := *entry
	clone.Value = append([]byte(nil), entry.Value...)
	return &clone
}

package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sanketn26/gossipcache/internal/storage"
)

// Clock returns the current time. Tests inject a fake clock to make TTL
// behavior deterministic without sleeping.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Options configures a MemoryStorage instance. Zero values for MaxKeySize and
// MaxValueSize disable enforcement of those limits.
type Options struct {
	MaxSize        int64
	EvictionPolicy string
	MaxKeySize     int
	MaxValueSize   int
	Clock          Clock
}

// MemoryStorage is an in-memory cache storage engine with optional TTL
// expiration and pluggable eviction policy.
type MemoryStorage struct {
	data         *shardedMap
	stats        *storageStats
	maxSize      int64
	maxKeySize   int
	maxValueSize int
	eviction     EvictionPolicy
	clock        Clock
	size         atomic.Int64
	closed       atomic.Bool
	closeCh      chan struct{}
	wg           sync.WaitGroup
}

type storageStats struct {
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

// New creates a new MemoryStorage from opts.
func New(opts Options) (*MemoryStorage, error) {
	eviction, err := newEvictionPolicy(opts.EvictionPolicy)
	if err != nil {
		return nil, err
	}

	clock := opts.Clock
	if clock == nil {
		clock = realClock{}
	}

	ms := &MemoryStorage{
		data:         newShardedMap(256),
		stats:        &storageStats{},
		maxSize:      opts.MaxSize,
		maxKeySize:   opts.MaxKeySize,
		maxValueSize: opts.MaxValueSize,
		eviction:     eviction,
		clock:        clock,
		closeCh:      make(chan struct{}),
	}

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

	if entry.IsExpiredAt(ms.clock.Now()) {
		removed := ms.data.delete(key)
		if removed > 0 {
			ms.size.Add(-removed)
			ms.eviction.OnRemove(key)
		}
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

	if ms.maxKeySize > 0 && len(key) > ms.maxKeySize {
		return fmt.Errorf("%w: %d bytes (limit %d)", storage.ErrKeyTooLarge, len(key), ms.maxKeySize)
	}
	if ms.maxValueSize > 0 && len(value) > ms.maxValueSize {
		return fmt.Errorf("%w: %d bytes (limit %d)", storage.ErrValueTooLarge, len(value), ms.maxValueSize)
	}

	now := ms.clock.Now()
	entry := &storage.Entry{
		Key:       key,
		Value:     append([]byte(nil), value...),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if ttl > 0 {
		entry.ExpiresAt = now.Add(ttl)
	}

	newSize := entrySize(entry)
	prevSize := ms.data.set(key, entry)
	ms.size.Add(newSize - prevSize)
	ms.eviction.OnAdd(key)

	for ms.size.Load() > ms.maxSize {
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

	removed := ms.data.delete(key)
	if removed > 0 {
		ms.size.Add(-removed)
		ms.eviction.OnRemove(key)
	}

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
	now := ms.clock.Now()
	for key, entry := range entries {
		var ttl time.Duration
		if !entry.ExpiresAt.IsZero() {
			ttl = entry.ExpiresAt.Sub(now)
			if ttl <= 0 {
				continue
			}
		}
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
		Size:      ms.size.Load(),
		Hits:      ms.stats.hits.Load(),
		Misses:    ms.stats.misses.Load(),
		Evictions: ms.stats.evictions.Load(),
	}, nil
}

func (ms *MemoryStorage) Close() error {
	if ms.closed.Swap(true) {
		return nil
	}

	close(ms.closeCh)
	ms.wg.Wait()

	return nil
}

func (ms *MemoryStorage) evict() error {
	victim := ms.eviction.SelectVictim()
	if victim == "" {
		return errors.New("no victim to evict")
	}

	removed := ms.data.delete(victim)
	if removed > 0 {
		ms.size.Add(-removed)
	}
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
	now := ms.clock.Now()
	keys := ms.data.keys()

	for _, key := range keys {
		entry, ok := ms.data.get(key)
		if ok && entry.IsExpiredAt(now) {
			removed := ms.data.delete(key)
			if removed > 0 {
				ms.size.Add(-removed)
				ms.eviction.OnRemove(key)
			}
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

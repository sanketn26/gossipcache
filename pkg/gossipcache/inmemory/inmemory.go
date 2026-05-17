// Package inmemory provides a public constructor for an in-memory
// gossipcache.Cache. It wires the internal cache manager and memory storage
// engine so library consumers do not need to depend on any internal package.
package inmemory

import (
	"time"

	"github.com/sanketn26/gossipcache/internal/cache"
	"github.com/sanketn26/gossipcache/internal/storage/memory"
	"github.com/sanketn26/gossipcache/pkg/gossipcache"
)

// Options configures the in-memory cache. Zero values for MaxKeySize and
// MaxValueSize disable enforcement of those limits.
type Options struct {
	// MaxSize is the soft byte cap of the cache. The engine evicts entries
	// once the running size exceeds this value.
	MaxSize int64

	// DefaultTTL is applied when callers pass ttl=0 to Set or SetMulti.
	DefaultTTL time.Duration

	// EvictionPolicy selects the eviction policy. Currently only "lru" is
	// supported; an empty value defaults to "lru".
	EvictionPolicy string

	// MaxKeySize, MaxValueSize cap individual key and value sizes. Zero
	// disables the corresponding check.
	MaxKeySize   int
	MaxValueSize int

	// Metrics, if non-nil, receives RecordCacheOperation and SetCacheStats
	// callbacks for every cache operation.
	Metrics MetricsRecorder

	// Clock, if non-nil, overrides the wall clock used for TTL expiry. Tests
	// inject a controllable clock to avoid sleeping.
	Clock Clock
}

// MetricsRecorder mirrors the cache-layer recorder so consumers can plug in
// their own metrics backend without importing internal packages.
type MetricsRecorder interface {
	RecordCacheOperation(operation string, err error)
	SetCacheStats(sizeBytes, keys int64)
}

// Clock is the time source used for TTL expiry checks. Production callers can
// ignore this; tests pass a fake clock to make expiry deterministic.
type Clock interface {
	Now() time.Time
}

// New constructs an in-memory gossipcache.Cache. Returned errors come from
// memory-engine construction (e.g. unsupported eviction policy).
func New(opts Options) (gossipcache.Cache, error) {
	if opts.EvictionPolicy == "" {
		opts.EvictionPolicy = "lru"
	}

	store, err := memory.New(memory.Options{
		MaxSize:        opts.MaxSize,
		EvictionPolicy: opts.EvictionPolicy,
		MaxKeySize:     opts.MaxKeySize,
		MaxValueSize:   opts.MaxValueSize,
		Clock:          memoryClock(opts.Clock),
	})
	if err != nil {
		return nil, err
	}

	cfg := &cache.CacheConfig{
		DefaultTTL: opts.DefaultTTL,
		Metrics:    cacheRecorder(opts.Metrics),
		Clock:      cacheClock(opts.Clock),
	}

	return cache.NewManager(store, cfg), nil
}

func memoryClock(c Clock) memory.Clock {
	if c == nil {
		return nil
	}
	return clockAdapter{c}
}

func cacheClock(c Clock) cache.Clock {
	if c == nil {
		return nil
	}
	return clockAdapter{c}
}

type clockAdapter struct{ Clock }

func cacheRecorder(r MetricsRecorder) cache.MetricsRecorder {
	if r == nil {
		return nil
	}
	return r
}

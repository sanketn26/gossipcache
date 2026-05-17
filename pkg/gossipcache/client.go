package gossipcache

import (
	"context"
	"time"
)

// Cache is the public client-facing interface for cache operations.
type Cache interface {
	// Get retrieves a value by key
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with TTL
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a key
	Delete(ctx context.Context, key string) error

	// GetMulti retrieves multiple keys
	GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)

	// SetMulti stores multiple entries
	SetMulti(ctx context.Context, entries map[string][]byte, ttl time.Duration) error

	// Flush removes all entries
	Flush(ctx context.Context) error

	// Stats returns cache statistics
	Stats(ctx context.Context) (*CacheStats, error)

	// Close gracefully shuts down the cache
	Close() error
}

// CacheStats represents cache statistics
type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int64
	Keys      int64
}

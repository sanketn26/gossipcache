package storage

import (
	"context"
	"time"
)

// Storage defines the interface for cache storage engines.
type Storage interface {
	// Get retrieves a value by key
	Get(ctx context.Context, key string) (*Entry, error)

	// Set stores a value with TTL
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a key
	Delete(ctx context.Context, key string) error

	// GetMulti retrieves multiple keys (batch operation)
	GetMulti(ctx context.Context, keys []string) (map[string]*Entry, error)

	// SetMulti stores multiple entries (batch operation)
	SetMulti(ctx context.Context, entries map[string]*Entry) error

	// Keys returns all keys (for anti-entropy)
	Keys(ctx context.Context) ([]string, error)

	// Stats returns storage statistics
	Stats(ctx context.Context) (*Stats, error)

	// Close cleans up resources
	Close() error
}

// Entry represents a cache entry
type Entry struct {
	Key       string
	Value     []byte
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Stats represents storage statistics
type Stats struct {
	Keys      int64
	Size      int64 // bytes
	Hits      int64
	Misses    int64
	Evictions int64
}

// IsExpiredAt reports whether the entry has expired relative to now. Callers
// that share an injected clock with storage pass it through here for
// deterministic expiry checks.
func (e *Entry) IsExpiredAt(now time.Time) bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return now.After(e.ExpiresAt)
}

// IsExpired reports whether the entry has expired against the real wall clock.
// Prefer IsExpiredAt in code that has access to an injected clock.
func (e *Entry) IsExpired() bool {
	return e.IsExpiredAt(time.Now())
}

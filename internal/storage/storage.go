package storage

import (
	"context"
	"time"
)

// Storage defines the interface for cache storage engines.
// SRP: Single responsibility - data storage and retrieval
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

// IsExpired checks if entry has expired
func (e *Entry) IsExpired() bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}

// Package backingstore defines the contract cache nodes use to talk to a
// persistent backing store (Redis/Valkey, Memcached, SQL, ...) in backed mode.
package backingstore

import (
	"context"
	"errors"
	"time"
)

// ErrKeyNotFound is returned by Get when the key does not exist in the
// backing store. Callers should test with errors.Is.
var ErrKeyNotFound = errors.New("backingstore: key not found")

// BackingStore defines the interface for persistent storage backends.
// Configuration is adapter-specific: each implementation package exports its
// own Config type so store-specific options never leak into this contract.
type BackingStore interface {
	// Get retrieves a value, its version, and its expiration time.
	// A zero ExpiresAt in the returned Entry means no expiration.
	// Returns ErrKeyNotFound if the key does not exist.
	Get(ctx context.Context, key string) (entry *Entry, err error)

	// Set stores a value with an optional TTL and returns the new version.
	// ttl == 0 means no expiration. ttl < 0 is invalid.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) (version int64, err error)

	// Delete removes a key. Delete is idempotent — deleting a missing key returns nil.
	Delete(ctx context.Context, key string) error

	// GetMulti retrieves multiple keys. Missing keys are omitted from the
	// result; any other per-key failure fails the whole call.
	GetMulti(ctx context.Context, keys []string) (map[string]*Entry, error)

	// SetMulti stores multiple entries, each with its own TTL.
	SetMulti(ctx context.Context, entries map[string]SetRequest) (map[string]int64, error)

	// Ping checks connectivity
	Ping(ctx context.Context) error

	// Close releases resources
	Close() error
}

// Entry represents a backing store entry.
// ExpiresAt is the absolute expiration time; zero value means no expiration.
type Entry struct {
	Key       string
	Value     []byte
	Version   int64
	ExpiresAt time.Time
}

// SetRequest carries the value and TTL for a single key in SetMulti.
// TTL == 0 means no expiration.
type SetRequest struct {
	Value []byte
	TTL   time.Duration
}

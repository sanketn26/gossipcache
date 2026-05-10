package gossipcache

import "errors"

var (
	// ErrKeyNotFound indicates the key was not found
	ErrKeyNotFound = errors.New("key not found")

	// ErrKeyTooLarge indicates the key exceeds maximum size
	ErrKeyTooLarge = errors.New("key too large")

	// ErrValueTooLarge indicates the value exceeds maximum size
	ErrValueTooLarge = errors.New("value too large")

	// ErrCacheFull indicates the cache is at capacity
	ErrCacheFull = errors.New("cache full")

	// ErrClosed indicates the cache is closed
	ErrClosed = errors.New("cache closed")
)

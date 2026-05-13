package storage

import "errors"

var (
	ErrNotFound    = errors.New("entry not found")
	ErrClosed      = errors.New("storage closed")
	ErrKeyNotFound = errors.New("key not found")
)

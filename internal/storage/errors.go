package storage

import "errors"

var (
	ErrClosed        = errors.New("storage closed")
	ErrKeyNotFound   = errors.New("key not found")
	ErrKeyTooLarge   = errors.New("key too large")
	ErrValueTooLarge = errors.New("value too large")
)

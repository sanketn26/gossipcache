package gossipcache

import (
	"context"
	"time"
)

var _ Cache = (*testCache)(nil)

type testCache struct{}

func (testCache) Get(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (testCache) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}

func (testCache) Delete(context.Context, string) error {
	return nil
}

func (testCache) GetMulti(context.Context, []string) (map[string][]byte, error) {
	return nil, nil
}

func (testCache) SetMulti(context.Context, map[string][]byte, time.Duration) error {
	return nil
}

func (testCache) Flush(context.Context) error {
	return nil
}

func (testCache) Stats(context.Context) (*CacheStats, error) {
	return nil, nil
}

func (testCache) Close() error {
	return nil
}

package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sanketn26/gossipcache/internal/storage"
	"github.com/sanketn26/gossipcache/pkg/gossipcache"
)

// Manager coordinates local L1 cache operations against an injected storage
// engine and translates internal storage errors into the public sentinels
// exported from pkg/gossipcache so callers can use errors.Is across the
// package boundary.
//
// This is the single-process storage path. Hybrid L1 state machine (VALID /
// STALE / FETCHING), L2 RPC, and invalidation streams are later phases
// (docs/impl/PHASE_PLAN.md P1+).
type Manager struct {
	storage storage.Storage
	config  *CacheConfig
}

var _ gossipcache.Cache = (*Manager)(nil)

// Clock returns the current time. Tests inject a fake clock alongside the
// matching storage clock to make TTL behavior deterministic without sleeping.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// MetricsRecorder is the minimal metrics surface the Manager depends on. The
// interface is consumer-owned here so observability implementations can be
// swapped without coupling internal/cache to a concrete metrics backend.
type MetricsRecorder interface {
	RecordCacheOperation(operation string, err error)
	SetCacheStats(sizeBytes, keys int64)
}

type CacheConfig struct {
	DefaultTTL time.Duration
	Clock      Clock
	Metrics    MetricsRecorder
}

func NewManager(storage storage.Storage, config *CacheConfig) *Manager {
	if config.Clock == nil {
		config.Clock = realClock{}
	}
	return &Manager{
		storage: storage,
		config:  config,
	}
}

func (m *Manager) Get(ctx context.Context, key string) ([]byte, error) {
	entry, err := m.storage.Get(ctx, key)
	if err != nil {
		out := translateErr("get", err)
		m.recordOp("get", out)
		return nil, out
	}

	m.recordOp("get", nil)
	return entry.Value, nil
}

func (m *Manager) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = m.config.DefaultTTL
	}

	if err := m.storage.Set(ctx, key, value, ttl); err != nil {
		out := translateErr("set", err)
		m.recordOp("set", out)
		return out
	}
	m.recordOp("set", nil)
	return nil
}

func (m *Manager) Delete(ctx context.Context, key string) error {
	if err := m.storage.Delete(ctx, key); err != nil {
		out := translateErr("delete", err)
		m.recordOp("delete", out)
		return out
	}
	m.recordOp("delete", nil)
	return nil
}

func (m *Manager) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	entries, err := m.storage.GetMulti(ctx, keys)
	if err != nil {
		out := translateErr("get_multi", err)
		m.recordOp("get_multi", out)
		return nil, out
	}

	result := make(map[string][]byte, len(entries))
	for k, entry := range entries {
		result[k] = entry.Value
	}

	m.recordOp("get_multi", nil)
	return result, nil
}

func (m *Manager) SetMulti(ctx context.Context, entries map[string][]byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = m.config.DefaultTTL
	}

	storageEntries := make(map[string]*storage.Entry, len(entries))
	now := m.config.Clock.Now()

	for k, v := range entries {
		storageEntries[k] = &storage.Entry{
			Key:       k,
			Value:     v,
			CreatedAt: now,
			UpdatedAt: now,
			ExpiresAt: now.Add(ttl),
		}
	}

	if err := m.storage.SetMulti(ctx, storageEntries); err != nil {
		out := translateErr("set_multi", err)
		m.recordOp("set_multi", out)
		return out
	}
	m.recordOp("set_multi", nil)
	return nil
}

func (m *Manager) Flush(ctx context.Context) error {
	keys, err := m.storage.Keys(ctx)
	if err != nil {
		out := translateErr("flush", err)
		m.recordOp("flush", out)
		return out
	}

	for _, key := range keys {
		if err := m.storage.Delete(ctx, key); err != nil {
			out := translateErr("flush", err)
			m.recordOp("flush", out)
			return out
		}
	}

	m.recordOp("flush", nil)
	return nil
}

func (m *Manager) Stats(ctx context.Context) (*gossipcache.CacheStats, error) {
	stats, err := m.storage.Stats(ctx)
	if err != nil {
		out := translateErr("stats", err)
		m.recordOp("stats", out)
		return nil, out
	}

	if m.config.Metrics != nil {
		m.config.Metrics.SetCacheStats(stats.Size, stats.Keys)
	}
	m.recordOp("stats", nil)

	return &gossipcache.CacheStats{
		Hits:      stats.Hits,
		Misses:    stats.Misses,
		Evictions: stats.Evictions,
		Size:      stats.Size,
		Keys:      stats.Keys,
	}, nil
}

func (m *Manager) Close() error {
	if err := m.storage.Close(); err != nil {
		out := translateErr("close", err)
		m.recordOp("close", out)
		return out
	}
	m.recordOp("close", nil)
	return nil
}

func (m *Manager) recordOp(op string, err error) {
	if m.config.Metrics == nil {
		return
	}
	m.config.Metrics.RecordCacheOperation(op, err)
}

// translateErr maps an internal storage error to its public counterpart so
// callers of pkg/gossipcache can use errors.Is against the exported sentinels.
// The operation label gives the caller context about which call surfaced the
// error without leaking internal package paths.
func translateErr(op string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, storage.ErrKeyNotFound):
		return fmt.Errorf("%s: %w", op, gossipcache.ErrKeyNotFound)
	case errors.Is(err, storage.ErrClosed):
		return fmt.Errorf("%s: %w", op, gossipcache.ErrClosed)
	case errors.Is(err, storage.ErrKeyTooLarge):
		return fmt.Errorf("%s: %w", op, gossipcache.ErrKeyTooLarge)
	case errors.Is(err, storage.ErrValueTooLarge):
		return fmt.Errorf("%s: %w", op, gossipcache.ErrValueTooLarge)
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

package cache

import (
	"context"
	"time"

	"github.com/sanketn26/gossipcache/internal/storage"
	"github.com/sanketn26/gossipcache/pkg/gossipcache"
)

// Manager implements the Cache interface
// SRP: Coordinates between storage and higher-level cache operations
type Manager struct {
	storage storage.Storage
	config  *CacheConfig
}

type CacheConfig struct {
	DefaultTTL time.Duration
}

func NewManager(storage storage.Storage, config *CacheConfig) *Manager {
	return &Manager{
		storage: storage,
		config:  config,
	}
}

func (m *Manager) Get(ctx context.Context, key string) ([]byte, error) {
	entry, err := m.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	return entry.Value, nil
}

func (m *Manager) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = m.config.DefaultTTL
	}

	return m.storage.Set(ctx, key, value, ttl)
}

func (m *Manager) Delete(ctx context.Context, key string) error {
	return m.storage.Delete(ctx, key)
}

func (m *Manager) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	entries, err := m.storage.GetMulti(ctx, keys)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]byte, len(entries))
	for k, entry := range entries {
		result[k] = entry.Value
	}

	return result, nil
}

func (m *Manager) SetMulti(ctx context.Context, entries map[string][]byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = m.config.DefaultTTL
	}

	storageEntries := make(map[string]*storage.Entry)
	now := time.Now()

	for k, v := range entries {
		storageEntries[k] = &storage.Entry{
			Key:       k,
			Value:     v,
			CreatedAt: now,
			UpdatedAt: now,
			ExpiresAt: now.Add(ttl),
		}
	}

	return m.storage.SetMulti(ctx, storageEntries)
}

func (m *Manager) Flush(ctx context.Context) error {
	keys, err := m.storage.Keys(ctx)
	if err != nil {
		return err
	}

	for _, key := range keys {
		if err := m.storage.Delete(ctx, key); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) Stats(ctx context.Context) (*gossipcache.CacheStats, error) {
	stats, err := m.storage.Stats(ctx)
	if err != nil {
		return nil, err
	}

	return &gossipcache.CacheStats{
		Hits:      stats.Hits,
		Misses:    stats.Misses,
		Evictions: stats.Evictions,
		Size:      stats.Size,
		Keys:      stats.Keys,
	}, nil
}

func (m *Manager) Close() error {
	return m.storage.Close()
}

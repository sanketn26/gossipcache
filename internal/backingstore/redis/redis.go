// Package redis implements backingstore.BackingStore using Redis (covers
// Valkey via API compatibility).
package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sanketn26/gossipcache/internal/backingstore"
)

// Config holds Redis-specific connection options.
type Config struct {
	Address  string
	Password string
	DB       int
	PoolSize int
	Timeout  time.Duration
}

// Store implements backingstore.BackingStore using Redis.
type Store struct {
	client *redis.Client
}

var _ backingstore.BackingStore = (*Store)(nil)

// setScript atomically bumps the version, writes the value, and applies or
// clears expiration in one round-trip. ARGV[2] is TTL in milliseconds; 0 means
// "no expiration" (clear any existing TTL).
var setScript = redis.NewScript(`
    local hashKey = KEYS[1]
    local value = ARGV[1]
    local ttlMs = tonumber(ARGV[2])

    local version = redis.call('HGET', hashKey, 'version')
    if not version then
        version = 0
    end
    version = tonumber(version) + 1

    redis.call('HSET', hashKey, 'value', value, 'version', version)

    if ttlMs > 0 then
        redis.call('PEXPIRE', hashKey, ttlMs)
    else
        redis.call('PERSIST', hashKey)
    end

    return version
`)

// New creates a new Redis backing store and verifies connectivity.
func New(ctx context.Context, cfg *Config) (*Store, error) {
	if cfg.Address == "" {
		return nil, errors.New("redis: address is required")
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  cfg.Timeout,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &Store{client: client}, nil
}

// NewFromClient wraps an existing client. Useful for tests and callers that
// manage their own client lifecycle; Close closes the provided client.
func NewFromClient(client *redis.Client) *Store {
	return &Store{client: client}
}

func (r *Store) Get(ctx context.Context, key string) (*backingstore.Entry, error) {
	// Value and version live in a Redis hash. PTTL gives remaining TTL in
	// milliseconds so the cache layer can populate ExpiresAt.
	hashKey := hashKey(key)

	pipe := r.client.Pipeline()
	hgetAll := pipe.HGetAll(ctx, hashKey)
	pttl := pipe.PTTL(ctx, hashKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("redis get %q: %w", key, err)
	}

	result, err := hgetAll.Result()
	if err != nil {
		return nil, fmt.Errorf("redis get %q: %w", key, err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("get %q: %w", key, backingstore.ErrKeyNotFound)
	}

	entry, err := entryFromHash(key, result)
	if err != nil {
		return nil, err
	}

	// PTTL returns -1 if the key has no expiration, -2 if missing.
	if ms, err := pttl.Result(); err == nil && ms > 0 {
		entry.ExpiresAt = time.Now().Add(ms)
	}
	return entry, nil
}

func (r *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) (int64, error) {
	ttlMs, err := ttlToMillis(ttl)
	if err != nil {
		return 0, err
	}

	result, err := setScript.Run(ctx, r.client, []string{hashKey(key)}, value, ttlMs).Result()
	if err != nil {
		return 0, fmt.Errorf("redis set %q: %w", key, err)
	}

	version, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("redis set %q: unexpected version type %T", key, result)
	}

	return version, nil
}

func (r *Store) Delete(ctx context.Context, key string) error {
	// DEL is idempotent: deleting a missing key is not an error. Note there is
	// no tombstone yet, so a delete is indistinguishable from "never existed"
	// to anti-entropy; tombstones are addressed with the gossip layer.
	return r.client.Del(ctx, hashKey(key)).Err()
}

func (r *Store) GetMulti(ctx context.Context, keys []string) (map[string]*backingstore.Entry, error) {
	result := make(map[string]*backingstore.Entry, len(keys))
	if len(keys) == 0 {
		return result, nil
	}

	pipe := r.client.Pipeline()
	type cmdPair struct {
		data *redis.MapStringStringCmd
		ttl  *redis.DurationCmd
	}
	cmds := make(map[string]cmdPair, len(keys))

	for _, key := range keys {
		hk := hashKey(key)
		cmds[key] = cmdPair{
			data: pipe.HGetAll(ctx, hk),
			ttl:  pipe.PTTL(ctx, hk),
		}
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("redis getmulti: %w", err)
	}

	now := time.Now()
	for key, c := range cmds {
		data, err := c.data.Result()
		if err != nil {
			// A real failure must surface; only a missing key (empty hash)
			// may be silently omitted from the result.
			return nil, fmt.Errorf("redis getmulti %q: %w", key, err)
		}
		if len(data) == 0 {
			continue
		}
		entry, err := entryFromHash(key, data)
		if err != nil {
			return nil, err
		}
		if d, err := c.ttl.Result(); err == nil && d > 0 {
			entry.ExpiresAt = now.Add(d)
		}
		result[key] = entry
	}

	return result, nil
}

func (r *Store) SetMulti(ctx context.Context, entries map[string]backingstore.SetRequest) (map[string]int64, error) {
	// Reuse the single-key Lua script per entry inside a pipeline so each key
	// gets the same atomic version-bump-plus-TTL behavior as Set.
	result := make(map[string]int64, len(entries))
	if len(entries) == 0 {
		return result, nil
	}

	ttls := make(map[string]int64, len(entries))
	for key, req := range entries {
		ttlMs, err := ttlToMillis(req.TTL)
		if err != nil {
			return nil, fmt.Errorf("setmulti %q: %w", key, err)
		}
		ttls[key] = ttlMs
	}

	pipe := r.client.Pipeline()
	pending := make(map[string]*redis.Cmd, len(entries))
	for key, req := range entries {
		pending[key] = setScript.Run(ctx, pipe, []string{hashKey(key)}, req.Value, ttls[key])
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("redis setmulti: %w", err)
	}

	for key, cmd := range pending {
		v, err := cmd.Int64()
		if err != nil {
			return nil, fmt.Errorf("redis setmulti version for %q: %w", key, err)
		}
		result[key] = v
	}
	return result, nil
}

func (r *Store) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *Store) Close() error {
	return r.client.Close()
}

func hashKey(key string) string {
	return "cache:" + key
}

// entryFromHash converts the stored hash fields into an Entry. A version that
// fails to parse is corruption and must not silently become version 0 — that
// would defeat version-comparison invalidation.
func entryFromHash(key string, fields map[string]string) (*backingstore.Entry, error) {
	version, err := strconv.ParseInt(fields["version"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("redis: corrupt version for key %q: %w", key, err)
	}
	return &backingstore.Entry{
		Key:     key,
		Value:   []byte(fields["value"]),
		Version: version,
	}, nil
}

// ttlToMillis validates a TTL and converts it to milliseconds for PEXPIRE.
// Sub-millisecond TTLs round up to 1ms: truncating to 0 would turn a short
// TTL into "no expiration", the opposite of the caller's intent.
func ttlToMillis(ttl time.Duration) (int64, error) {
	if ttl < 0 {
		return 0, fmt.Errorf("invalid negative ttl: %v", ttl)
	}
	if ttl == 0 {
		return 0, nil
	}
	ms := int64(ttl / time.Millisecond)
	if ms == 0 {
		ms = 1
	}
	return ms, nil
}

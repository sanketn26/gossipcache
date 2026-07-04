# Phase 2: Backed Mode Implementation

**Goal**: Implement backed mode with Redis and Memcached support, metadata gossip, and change detection/pull mechanism.

**Duration**: 3-4 weeks

**Prerequisites**: Phase 1 complete

**Status**: Not Started

## Overview

Phase 2 builds the distributed cache functionality for backed mode. We'll implement the backing store abstraction with two concrete adapters — Redis (covers Valkey via API compatibility) and Memcached — to stress-test the interface against fundamentally different stores. On top of that we build the gossip protocol for metadata propagation and the pull-based data synchronization mechanism.

Implementing Redis and Memcached together in Phase 2 is deliberate: Redis offers atomic Lua-scripted versioning, while Memcached has no Lua and no native version field. Forcing the `BackingStore` interface to accommodate both up front prevents Redis-shaped assumptions from leaking into the abstraction before Postgres/MySQL arrive in Phase 4.

## Objectives

- [ ] Backing store abstraction and interface (with TTL support)
- [ ] Redis connector with connection pooling, atomic version+TTL via Lua (covers Valkey)
- [ ] Memcached connector with `cas`-based versioning and exptime-quirk handling
- [ ] Metadata gossip protocol implementation
- [ ] Change detection and pull mechanism
- [ ] Singleflight pattern for thundering herd prevention
- [ ] Network layer (TCP/UDP)
- [ ] Gossip engine with peer management
- [ ] Anti-entropy synchronization
- [ ] Node discovery (static peers for now)
- [ ] Multi-node integration tests
- [ ] Performance benchmarks for distributed scenarios

## Architecture Reference

Review these documents before starting:
- [Backed Mode Sequences](../diagrams/BACKED_MODE_SEQUENCES.md)
- [Architecture - Backed Mode](../ARCHITECTURE.md#backed-mode)
- [Technical Spec - Backing Store](../TECHNICAL_SPEC.md#42-backing-store-interface)

## Package Structure Updates

```
gossipcache/
├── internal/
│   ├── backingstore/
│   │   ├── backingstore.go        # Interface
│   │   ├── redis/
│   │   │   ├── redis.go           # Redis implementation (covers Valkey)
│   │   │   └── redis_test.go
│   │   ├── memcached/
│   │   │   ├── memcached.go       # Memcached implementation (cas-based versioning)
│   │   │   └── memcached_test.go
│   │   └── mock/
│   │       └── mock_backingstore.go
│   ├── gossip/
│   │   ├── engine.go              # Gossip engine
│   │   ├── message.go             # Message types
│   │   ├── protocol.go            # Protocol logic
│   │   ├── peer.go                # Peer management
│   │   └── antientropy.go         # Anti-entropy
│   ├── network/
│   │   ├── transport.go           # TCP/UDP transport
│   │   ├── codec.go               # Message encoding/decoding
│   │   └── discovery.go           # Node discovery
│   ├── vclock/                    # (Phase 3)
│   └── util/
│       └── singleflight.go        # DRY: Shared singleflight
└── test/
    └── integration/
        ├── backed_mode_test.go
        └── multi_node_test.go
```

## Implementation Steps

### Phase 2 TDD Rhythm

Start each feature with a deterministic unit test and add Redis/network integration only after the unit contract is stable. Use fakes for backing stores, peers, and clocks so metadata gossip can be tested without a live cluster first.

### Step 1: Backing Store Interface (Day 1-2)

**SOLID**: Interface Segregation - Clean interface for backing stores

#### 1.1 Define Backing Store Interface

```go
// internal/backingstore/backingstore.go
package backingstore

import (
    "context"
    "time"
)

// BackingStore defines the interface for persistent storage backends
// ISP: Focused interface for backing store operations
type BackingStore interface {
    // Get retrieves a value, its version, and its expiration time.
    // A zero ExpiresAt in the returned Entry means no expiration.
    Get(ctx context.Context, key string) (entry *Entry, err error)

    // Set stores a value with an optional TTL and returns the new version.
    // ttl == 0 means no expiration. ttl < 0 is invalid.
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) (version int64, err error)

    // Delete removes a key. Delete is idempotent — deleting a missing key returns nil.
    Delete(ctx context.Context, key string) error

    // GetMulti retrieves multiple keys. Missing keys are omitted from the result.
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

// Config holds backing store configuration
type Config struct {
    Type     string // "redis", "memcached", "postgres", etc.
    Address  string
    Database string
    Username string
    Password string
    PoolSize int
    Timeout  time.Duration
}
```

#### 1.2 Versioning Strategy Per Store

The interface returns a monotonic `int64` version on every `Set`. Each adapter is responsible for producing one — the mechanism differs by store and is the main thing the interface has to stay neutral about:

| Store | Versioning Mechanism |
| --- | --- |
| Redis / Valkey | Lua `EVAL` script does `HINCRBY version 1` + `HSET value` atomically in one round-trip |
| Memcached | Native `cas` token from `gets`; adapter does `gets` → increment → `cas` with retry on token mismatch. Value bytes are stored as `[8-byte version][payload]` so `Get` can split them in one op |
| Postgres / MySQL (Phase 4) | `UPDATE ... SET version = version + 1 RETURNING version` in a transaction |

Test the interface against a fake plus both real adapters in Phase 2. If a future adapter cannot produce a monotonic version (e.g., a store without CAS), that is a signal to extend the interface — do not paper over it inside the adapter.

#### 1.3 TTL: Backing Store Is Source of Truth

The interface accepts a `ttl time.Duration` on `Set`/`SetMulti` and returns an `ExpiresAt time.Time` on `Get`/`GetMulti`. The cache layer must treat the backing store as authoritative for expiration:

- On `Set(key, value, ttl)`: the adapter persists the value with the given TTL using the store's native expiration mechanism (Redis `SET EX`, Memcached `exptime`, Postgres `expires_at` column + sweeper).
- On `Get`: if the store returns the entry, the adapter populates `ExpiresAt` from the store. If the store has already expired the key, `Get` returns `ErrKeyNotFound` — the cache treats this identically to a real miss.
- The local cache, when populating from a backing-store pull, sets its own per-entry expiry to `min(configured_local_ttl, ExpiresAt - now)`. This guarantees a node never serves a key past the backing store's expiration, even if local TTL would have kept it longer.

| TTL semantics | Meaning |
| --- | --- |
| `ttl == 0` | No expiration. `ExpiresAt` in returned `Entry` is the zero `time.Time`. |
| `ttl > 0` | Key expires after `ttl` from the moment of the write. |
| `ttl < 0` | Invalid — adapters must return an error. |

Per-store mapping:

| Store | TTL Mechanism |
| --- | --- |
| Redis / Valkey | `SET key value EX <seconds>` (sub-second TTL rounds up to 1s); or `PEXPIRE` for millisecond precision in the Lua script |
| Memcached | `exptime` parameter. **Quirk**: ≤ 30 days = relative seconds; > 30 days = absolute Unix timestamp. Adapter must convert. |
| Postgres / MySQL (Phase 4) | `expires_at TIMESTAMP` column; a background sweeper deletes expired rows. Reads filter `WHERE expires_at IS NULL OR expires_at > now()`. |

### Step 2: Redis Connector (Day 2-5)

**SOLID**: Single Responsibility - Redis connector handles only Redis communication

#### 2.1 Redis Implementation

```go
// internal/backingstore/redis/redis.go
package redis

import (
    "context"
    "fmt"
    "strconv"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/sanketn26/gossipcache/internal/backingstore"
)

// RedisStore implements BackingStore using Redis
type RedisStore struct {
    client  *redis.Client
    timeout time.Duration
}

// New creates a new Redis backing store
func New(cfg *backingstore.Config) (*RedisStore, error) {
    opts := &redis.Options{
        Addr:         cfg.Address,
        Password:     cfg.Password,
        DB:           parseDB(cfg.Database),
        PoolSize:     cfg.PoolSize,
        DialTimeout:  cfg.Timeout,
        ReadTimeout:  cfg.Timeout,
        WriteTimeout: cfg.Timeout,
    }

    client := redis.NewClient(opts)

    // Test connection
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := client.Ping(ctx).Err(); err != nil {
        return nil, fmt.Errorf("redis ping failed: %w", err)
    }

    return &RedisStore{
        client:  client,
        timeout: cfg.Timeout,
    }, nil
}

func (r *RedisStore) Get(ctx context.Context, key string) (*backingstore.Entry, error) {
    // Use Redis hash to store value and version. PTTL gives remaining TTL in
    // milliseconds so the cache layer can populate ExpiresAt.
    hashKey := fmt.Sprintf("cache:%s", key)

    pipe := r.client.Pipeline()
    hgetAll := pipe.HGetAll(ctx, hashKey)
    pttl := pipe.PTTL(ctx, hashKey)
    if _, err := pipe.Exec(ctx); err != nil {
        return nil, err
    }

    result, err := hgetAll.Result()
    if err != nil {
        return nil, err
    }
    if len(result) == 0 {
        return nil, backingstore.ErrKeyNotFound
    }

    version, _ := strconv.ParseInt(result["version"], 10, 64)
    entry := &backingstore.Entry{
        Key:     key,
        Value:   []byte(result["value"]),
        Version: version,
    }

    // PTTL returns -1 if the key has no expiration, -2 if missing.
    if ms, err := pttl.Result(); err == nil && ms > 0 {
        entry.ExpiresAt = time.Now().Add(time.Duration(ms) * time.Millisecond)
    }
    return entry, nil
}

func (r *RedisStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) (int64, error) {
    if ttl < 0 {
        return 0, fmt.Errorf("invalid negative ttl: %v", ttl)
    }
    hashKey := fmt.Sprintf("cache:%s", key)

    // Atomic: bump version, write value, apply or clear expiration in one round-trip.
    // ARGV[2] is TTL in milliseconds; 0 means "no expiration" (clear any existing TTL).
    script := redis.NewScript(`
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

    ttlMs := int64(ttl / time.Millisecond)
    result, err := script.Run(ctx, r.client, []string{hashKey}, value, ttlMs).Result()
    if err != nil {
        return 0, err
    }

    version, ok := result.(int64)
    if !ok {
        return 0, fmt.Errorf("unexpected version type: %T", result)
    }

    return version, nil
}

func (r *RedisStore) Delete(ctx context.Context, key string) error {
    hashKey := fmt.Sprintf("cache:%s", key)
    return r.client.Del(ctx, hashKey).Err()
}

func (r *RedisStore) GetMulti(ctx context.Context, keys []string) (map[string]*backingstore.Entry, error) {
    result := make(map[string]*backingstore.Entry)

    pipe := r.client.Pipeline()
    type cmdPair struct {
        data *redis.MapStringStringCmd
        ttl  *redis.DurationCmd
    }
    cmds := make(map[string]cmdPair, len(keys))

    for _, key := range keys {
        hashKey := fmt.Sprintf("cache:%s", key)
        cmds[key] = cmdPair{
            data: pipe.HGetAll(ctx, hashKey),
            ttl:  pipe.PTTL(ctx, hashKey),
        }
    }

    if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
        return nil, err
    }

    now := time.Now()
    for key, c := range cmds {
        data, err := c.data.Result()
        if err != nil || len(data) == 0 {
            continue
        }
        version, _ := strconv.ParseInt(data["version"], 10, 64)
        entry := &backingstore.Entry{
            Key:     key,
            Value:   []byte(data["value"]),
            Version: version,
        }
        if d, err := c.ttl.Result(); err == nil && d > 0 {
            entry.ExpiresAt = now.Add(d)
        }
        result[key] = entry
    }

    return result, nil
}

func (r *RedisStore) SetMulti(ctx context.Context, entries map[string]backingstore.SetRequest) (map[string]int64, error) {
    // Reuse the single-key Lua script per entry inside a pipeline so each
    // key gets the same atomic version-bump-plus-TTL behavior as Set.
    result := make(map[string]int64, len(entries))
    if len(entries) == 0 {
        return result, nil
    }

    script := redis.NewScript(`
        local hashKey = KEYS[1]
        local value = ARGV[1]
        local ttlMs = tonumber(ARGV[2])

        local version = redis.call('HGET', hashKey, 'version')
        if not version then version = 0 end
        version = tonumber(version) + 1

        redis.call('HSET', hashKey, 'value', value, 'version', version)
        if ttlMs > 0 then
            redis.call('PEXPIRE', hashKey, ttlMs)
        else
            redis.call('PERSIST', hashKey)
        end
        return version
    `)

    pipe := r.client.Pipeline()
    type pending struct {
        cmd *redis.Cmd
    }
    pendings := make(map[string]pending, len(entries))

    for key, req := range entries {
        if req.TTL < 0 {
            return nil, fmt.Errorf("invalid negative ttl for key %q: %v", key, req.TTL)
        }
        hashKey := fmt.Sprintf("cache:%s", key)
        ttlMs := int64(req.TTL / time.Millisecond)
        pendings[key] = pending{cmd: script.Run(ctx, pipe, []string{hashKey}, req.Value, ttlMs)}
    }

    if _, err := pipe.Exec(ctx); err != nil {
        return nil, err
    }

    for key, p := range pendings {
        v, err := p.cmd.Int64()
        if err != nil {
            return result, fmt.Errorf("setmulti version for %q: %w", key, err)
        }
        result[key] = v
    }
    return result, nil
}

func (r *RedisStore) Ping(ctx context.Context) error {
    return r.client.Ping(ctx).Err()
}

func (r *RedisStore) Close() error {
    return r.client.Close()
}

func parseDB(db string) int {
    parsed, _ := strconv.Atoi(db)
    return parsed
}
```

#### 2.2 Redis Tests

```go
// internal/backingstore/redis/redis_test.go
package redis

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/sanketn26/gossipcache/internal/backingstore"
)

func TestRedisStore_GetSet(t *testing.T) {
    // Skip if no Redis available
    if testing.Short() {
        t.Skip("Skipping Redis integration test")
    }

    store, err := New(&backingstore.Config{
        Address:  "localhost:6379",
        Database: "0",
        PoolSize: 10,
        Timeout:  5 * time.Second,
    })
    require.NoError(t, err)
    defer store.Close()

    ctx := context.Background()

    // Test Set with no TTL
    version1, err := store.Set(ctx, "test_key", []byte("test_value"), 0)
    require.NoError(t, err)
    require.Greater(t, version1, int64(0))

    // Test Get
    entry, err := store.Get(ctx, "test_key")
    require.NoError(t, err)
    require.Equal(t, []byte("test_value"), entry.Value)
    require.Equal(t, version1, entry.Version)
    require.True(t, entry.ExpiresAt.IsZero(), "no TTL set, ExpiresAt should be zero")

    // Test Update increments version
    version2, err := store.Set(ctx, "test_key", []byte("updated_value"), 0)
    require.NoError(t, err)
    require.Greater(t, version2, version1)

    // Test Set with TTL surfaces ExpiresAt
    _, err = store.Set(ctx, "ttl_key", []byte("v"), 30*time.Second)
    require.NoError(t, err)
    ttlEntry, err := store.Get(ctx, "ttl_key")
    require.NoError(t, err)
    require.WithinDuration(t, time.Now().Add(30*time.Second), ttlEntry.ExpiresAt, 2*time.Second)

    // Cleanup
    store.Delete(ctx, "test_key")
    store.Delete(ctx, "ttl_key")
}

func TestRedisStore_TTLExpires(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping Redis integration test")
    }
    store, err := New(&backingstore.Config{
        Address:  "localhost:6379",
        Database: "0",
        PoolSize: 10,
        Timeout:  5 * time.Second,
    })
    require.NoError(t, err)
    defer store.Close()

    ctx := context.Background()
    _, err = store.Set(ctx, "expiring_key", []byte("v"), 1*time.Second)
    require.NoError(t, err)

    // Wait past TTL; Redis should report the key as missing.
    time.Sleep(1500 * time.Millisecond)
    _, err = store.Get(ctx, "expiring_key")
    require.ErrorIs(t, err, backingstore.ErrKeyNotFound)
}

func TestRedisStore_RejectsNegativeTTL(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping Redis integration test")
    }
    store, err := New(&backingstore.Config{Address: "localhost:6379", Timeout: 5 * time.Second})
    require.NoError(t, err)
    defer store.Close()

    _, err = store.Set(context.Background(), "k", []byte("v"), -1*time.Second)
    require.Error(t, err)
}
```

### Step 2.5: Memcached Connector (Day 4-5)

**SOLID**: Single Responsibility — Memcached connector handles only Memcached communication. Liskov Substitution — Memcached's `Get`/`Set`/`Delete` must be observationally interchangeable with Redis behind `BackingStore`.

**Why now**: Memcached has no Lua, no hash type, and no server-side version counter. Building it alongside Redis catches any Redis-specific assumptions baked into the `BackingStore` interface before they harden.

#### 2.5.1 Versioning via `cas` + Value Framing

Memcached exposes a per-key `cas` (compare-and-swap) token that changes on every successful write. We use it as the optimistic-concurrency primitive, but the *version* the interface returns is a separate monotonic counter we maintain ourselves by framing the stored bytes:

```
stored value = [ 8-byte big-endian version ][ raw payload ]
```

`Set` flow:
1. `gets <key>` → returns `[version || payload]` and `cas` token, or miss
2. Compute `newVersion = oldVersion + 1` (or `1` on miss)
3. `cas <key> <flags> <exp> <len> <cas-token>` with framed `[newVersion || newPayload]`, where `<exp>` encodes the TTL (see below)
4. On `EXISTS` response (token mismatch → concurrent write), retry from step 1 up to N times
5. On `NOT_FOUND` (key vanished), fall back to `add`; if `add` says `NOT_STORED`, retry

This gives the same `(value, version)` contract as Redis without requiring server-side scripting.

#### 2.5.1.5 The Memcached TTL Quirk

Memcached's `exptime` field is overloaded:

- `0` → no expiration
- `1..2592000` (≤ 30 days, in seconds) → **relative** seconds from now
- `> 2592000` → **absolute** Unix timestamp

If you naively pass `int32(ttl.Seconds())` for a TTL of 40 days, memcached interprets it as a Unix timestamp far in the past and the key expires immediately. The adapter must convert:

```go
const memcachedRelativeThreshold = 30 * 24 * time.Hour // 2592000 seconds

func encodeExptime(ttl time.Duration, now time.Time) int32 {
    if ttl <= 0 {
        return 0 // no expiration
    }
    if ttl <= memcachedRelativeThreshold {
        // Sub-second TTLs round up to 1 second (memcached has no finer granularity).
        secs := int64(ttl / time.Second)
        if ttl%time.Second != 0 {
            secs++
        }
        return int32(secs)
    }
    return int32(now.Add(ttl).Unix())
}
```

Because the relative form uses *seconds*, the adapter also returns `ExpiresAt` rounded to second granularity — finer precision is not available from memcached. Callers needing sub-second TTL should pick Redis instead.

#### 2.5.2 Memcached Implementation

```go
// internal/backingstore/memcached/memcached.go
package memcached

import (
    "context"
    "encoding/binary"
    "errors"
    "fmt"
    "time"

    "github.com/bradfitz/gomemcache/memcache"
    "github.com/sanketn26/gossipcache/internal/backingstore"
)

const (
    versionPrefixLen = 8
    maxCASRetries    = 5
)

// Store implements BackingStore using Memcached.
type Store struct {
    client *memcache.Client
}

// New creates a new Memcached backing store. Address may be a comma-separated
// list "host1:11211,host2:11211" for client-side sharding across servers.
func New(cfg *backingstore.Config) (*Store, error) {
    client := memcache.New(cfg.Address)
    client.Timeout = cfg.Timeout
    if cfg.PoolSize > 0 {
        client.MaxIdleConns = cfg.PoolSize
    }
    if err := client.Ping(); err != nil {
        return nil, fmt.Errorf("memcached ping failed: %w", err)
    }
    return &Store{client: client}, nil
}

func (s *Store) Get(ctx context.Context, key string) (*backingstore.Entry, error) {
    item, err := s.client.Get(key)
    if errors.Is(err, memcache.ErrCacheMiss) {
        return nil, backingstore.ErrKeyNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("memcached get %q: %w", key, err)
    }
    version, value, err := unframe(item.Value)
    if err != nil {
        return nil, err
    }
    // Return a defensive copy — adapter must not hand out client-owned buffers.
    out := make([]byte, len(value))
    copy(out, value)
    return &backingstore.Entry{
        Key:       key,
        Value:     out,
        Version:   version,
        ExpiresAt: decodeExpiration(item.Expiration, time.Now()),
    }, nil
}

func (s *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) (int64, error) {
    if ttl < 0 {
        return 0, fmt.Errorf("invalid negative ttl: %v", ttl)
    }
    exptime := encodeExptime(ttl, time.Now())

    for attempt := 0; attempt < maxCASRetries; attempt++ {
        item, err := s.client.Get(key)
        switch {
        case errors.Is(err, memcache.ErrCacheMiss):
            // First write — use Add so we fail if a concurrent writer beat us.
            addErr := s.client.Add(&memcache.Item{
                Key:        key,
                Value:      frame(1, value),
                Expiration: exptime,
            })
            if errors.Is(addErr, memcache.ErrNotStored) {
                continue // racing first-write; retry as CAS path
            }
            if addErr != nil {
                return 0, fmt.Errorf("memcached add %q: %w", key, addErr)
            }
            return 1, nil
        case err != nil:
            return 0, fmt.Errorf("memcached gets %q: %w", key, err)
        }

        oldVersion, _, err := unframe(item.Value)
        if err != nil {
            return 0, err
        }
        newVersion := oldVersion + 1
        item.Value = frame(newVersion, value)
        item.Expiration = exptime // refresh or clear expiration on update

        casErr := s.client.CompareAndSwap(item)
        if errors.Is(casErr, memcache.ErrCASConflict) {
            continue // concurrent writer won; retry
        }
        if errors.Is(casErr, memcache.ErrNotStored) {
            continue // key evicted between gets and cas; retry
        }
        if casErr != nil {
            return 0, fmt.Errorf("memcached cas %q: %w", key, casErr)
        }
        return newVersion, nil
    }
    return 0, fmt.Errorf("memcached set %q: cas conflict after %d retries", key, maxCASRetries)
}

func (s *Store) Delete(ctx context.Context, key string) error {
    err := s.client.Delete(key)
    if errors.Is(err, memcache.ErrCacheMiss) {
        return nil // delete is idempotent
    }
    if err != nil {
        return fmt.Errorf("memcached delete %q: %w", key, err)
    }
    return nil
}

func (s *Store) GetMulti(ctx context.Context, keys []string) (map[string]*backingstore.Entry, error) {
    items, err := s.client.GetMulti(keys)
    if err != nil {
        return nil, fmt.Errorf("memcached getmulti: %w", err)
    }
    now := time.Now()
    out := make(map[string]*backingstore.Entry, len(items))
    for k, item := range items {
        version, value, err := unframe(item.Value)
        if err != nil {
            return nil, err
        }
        copied := make([]byte, len(value))
        copy(copied, value)
        out[k] = &backingstore.Entry{
            Key:       k,
            Value:     copied,
            Version:   version,
            ExpiresAt: decodeExpiration(item.Expiration, now),
        }
    }
    return out, nil
}

func (s *Store) SetMulti(ctx context.Context, entries map[string]backingstore.SetRequest) (map[string]int64, error) {
    // Memcached has no atomic multi-set. Do per-key Set so each gets a
    // correct monotonic version (and per-key TTL) under the same cas semantics
    // as single-key.
    out := make(map[string]int64, len(entries))
    for k, req := range entries {
        version, err := s.Set(ctx, k, req.Value, req.TTL)
        if err != nil {
            return out, fmt.Errorf("setmulti %q: %w", k, err)
        }
        out[k] = version
    }
    return out, nil
}

func (s *Store) Ping(ctx context.Context) error {
    return s.client.Ping()
}

func (s *Store) Close() error {
    // gomemcache has no explicit Close; idle conns are GC'd. Keep the method
    // for interface compliance and future client swaps.
    return nil
}

func frame(version int64, payload []byte) []byte {
    buf := make([]byte, versionPrefixLen+len(payload))
    binary.BigEndian.PutUint64(buf[:versionPrefixLen], uint64(version))
    copy(buf[versionPrefixLen:], payload)
    return buf
}

func unframe(framed []byte) (int64, []byte, error) {
    if len(framed) < versionPrefixLen {
        return 0, nil, fmt.Errorf("memcached value too short: %d bytes", len(framed))
    }
    version := int64(binary.BigEndian.Uint64(framed[:versionPrefixLen]))
    return version, framed[versionPrefixLen:], nil
}

// memcachedRelativeThreshold is the cutoff (in the protocol) between
// "exptime is relative seconds" and "exptime is an absolute Unix timestamp".
const memcachedRelativeThreshold = 30 * 24 * time.Hour // 2_592_000 seconds

// encodeExptime converts a Go duration to memcached's overloaded exptime field.
// 0 means no expiration. TTLs <= 30 days are encoded as relative seconds;
// larger TTLs are encoded as an absolute Unix timestamp.
func encodeExptime(ttl time.Duration, now time.Time) int32 {
    if ttl <= 0 {
        return 0
    }
    if ttl <= memcachedRelativeThreshold {
        secs := int64(ttl / time.Second)
        if ttl%time.Second != 0 {
            secs++ // round sub-second TTLs up to 1s — memcached has no finer granularity
        }
        return int32(secs)
    }
    return int32(now.Add(ttl).Unix())
}

// decodeExpiration converts the exptime stored in a memcached Item back to an
// absolute time. Memcached does not return remaining TTL on Get, so we
// reconstruct from the stored exptime (which gomemcache populates on returned
// Items). 0 means no expiration. Values <= 30 days are interpreted as already
// being absolute by the time we read them back from the client library.
func decodeExpiration(exptime int32, now time.Time) time.Time {
    if exptime == 0 {
        return time.Time{}
    }
    if exptime <= int32(memcachedRelativeThreshold/time.Second) {
        // Item was stored with relative seconds; we cannot recover the original
        // store-time from memcached, so this is best-effort. The cache layer
        // should treat ExpiresAt as a lower bound and rely on memcached itself
        // for actual eviction.
        return now.Add(time.Duration(exptime) * time.Second)
    }
    return time.Unix(int64(exptime), 0)
}

// Context plumbing: gomemcache does not accept context.Context. Honor
// cancellation at call boundaries by checking ctx.Err() before each round-trip.
// A context-aware client (e.g., rainycape/memcache) can be swapped behind this
// adapter without changing callers.
var _ = time.Second
```

> **Note on context.Context**: `gomemcache` predates Go's `context` package and does not accept `ctx` in its method signatures. The adapter accepts `ctx` to satisfy the interface and should check `ctx.Err()` before each network round-trip (omitted above for brevity). If strict context propagation matters, swap the client for a context-aware fork behind the same adapter — callers are unaffected.

#### 2.5.3 Memcached Tests

Unit tests should cover framing, CAS retry behavior, and not-found semantics using an in-process fake. Integration tests use a real memcached on `localhost:11211`.

```go
// internal/backingstore/memcached/memcached_test.go
package memcached

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "github.com/sanketn26/gossipcache/internal/backingstore"
)

func TestMemcachedStore_SetIncrementsVersion(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping memcached integration test")
    }
    store, err := New(&backingstore.Config{
        Address:  "localhost:11211",
        PoolSize: 10,
        Timeout:  5 * time.Second,
    })
    require.NoError(t, err)
    defer store.Close()

    ctx := context.Background()
    t.Cleanup(func() { _ = store.Delete(ctx, "test_key") })

    v1, err := store.Set(ctx, "test_key", []byte("v1"), 0)
    require.NoError(t, err)
    require.Equal(t, int64(1), v1)

    entry, err := store.Get(ctx, "test_key")
    require.NoError(t, err)
    require.Equal(t, []byte("v1"), entry.Value)
    require.Equal(t, int64(1), entry.Version)
    require.True(t, entry.ExpiresAt.IsZero())

    v2, err := store.Set(ctx, "test_key", []byte("v2"), 0)
    require.NoError(t, err)
    require.Equal(t, int64(2), v2)
}

func TestMemcachedStore_GetMissingKey(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping memcached integration test")
    }
    store, err := New(&backingstore.Config{
        Address: "localhost:11211",
        Timeout: 5 * time.Second,
    })
    require.NoError(t, err)
    defer store.Close()

    _, err = store.Get(context.Background(), "definitely_missing_key")
    require.ErrorIs(t, err, backingstore.ErrKeyNotFound)
}

func TestMemcachedStore_TTLExpires(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping memcached integration test")
    }
    store, err := New(&backingstore.Config{Address: "localhost:11211", Timeout: 5 * time.Second})
    require.NoError(t, err)
    defer store.Close()

    ctx := context.Background()
    _, err = store.Set(ctx, "expiring_key", []byte("v"), 1*time.Second)
    require.NoError(t, err)

    time.Sleep(2 * time.Second) // memcached TTL granularity is 1s
    _, err = store.Get(ctx, "expiring_key")
    require.ErrorIs(t, err, backingstore.ErrKeyNotFound)
}

func TestEncodeExptime_RelativeAndAbsolute(t *testing.T) {
    now := time.Unix(1_700_000_000, 0)

    // Zero TTL → no expiration
    require.Equal(t, int32(0), encodeExptime(0, now))

    // Sub-second rounds up to 1
    require.Equal(t, int32(1), encodeExptime(500*time.Millisecond, now))

    // <= 30 days → relative seconds
    require.Equal(t, int32(60), encodeExptime(60*time.Second, now))
    require.Equal(t, int32(30*24*3600), encodeExptime(30*24*time.Hour, now))

    // > 30 days → absolute unix timestamp
    got := encodeExptime(31*24*time.Hour, now)
    require.Equal(t, int32(now.Add(31*24*time.Hour).Unix()), got)
}

func TestEncodeExptime_RejectsNegative(t *testing.T) {
    // Negative TTL must be rejected at the Set boundary, not silently
    // mapped to "no expiration".
    if testing.Short() {
        t.Skip("Skipping memcached integration test")
    }
    store, err := New(&backingstore.Config{Address: "localhost:11211", Timeout: 5 * time.Second})
    require.NoError(t, err)
    defer store.Close()

    _, err = store.Set(context.Background(), "k", []byte("v"), -1*time.Second)
    require.Error(t, err)
}

func TestMemcachedStore_CopyOnGet(t *testing.T) {
    // Storage must not return a slice the caller can mutate to corrupt
    // internal state — verify Get returns an independent copy.
    // (Implementation uses an in-process fake; details omitted.)
}
```

#### 2.5.4 Operational Notes

- **Eviction**: Memcached is LRU by default and *will* evict keys under memory pressure, regardless of TTL. Cache misses are normal and the gossip+pull mechanism handles them — but operators should size the memcached fleet for working-set capacity, not just hot keys.
- **No persistence**: Memcached is in-memory only. After a full memcached restart, all versions and TTLs reset. The cluster will detect this as a mass invalidation through anti-entropy; callers should treat memcached-backed mode as accepting cold-start refetches.
- **Sharding**: `gomemcache` does consistent-hashing client-side across the address list. All GossipCache nodes must be configured with the same server list in the same order.
- **TTL precision**: Memcached's TTL granularity is **one second**. Sub-second TTLs round up to 1s. `ExpiresAt` returned from `Get` is best-effort — memcached itself is the authoritative source of expiration, and the cache layer should not rely on `ExpiresAt` for precise eviction timing. If sub-second TTLs are required, use Redis.

### Step 3: Gossip Message Types (Day 5-6)

**SOLID**: Open/Closed - Message types can be extended without modifying core

#### 3.1 Message Definitions

```go
// internal/gossip/message.go
package gossip

import (
    "encoding/binary"
    "fmt"
    "time"
)

// MessageType represents the type of gossip message
type MessageType uint16

const (
    MsgChangeNotification MessageType = 1
    MsgAntiEntropyReq     MessageType = 3
    MsgAntiEntropyResp    MessageType = 4
    MsgJoinRequest        MessageType = 5
    MsgJoinAck            MessageType = 6
    MsgPing               MessageType = 9
    MsgAck                MessageType = 10
)

// Message is the interface for all gossip messages
type Message interface {
    Type() MessageType
    Serialize() ([]byte, error)
}

// ChangeNotification notifies peers of a key change (backed mode)
type ChangeNotification struct {
    Key       string
    Version   int64
    Checksum  string    // SHA256 of value
    Timestamp time.Time
    NodeID    string
}

func (m *ChangeNotification) Type() MessageType {
    return MsgChangeNotification
}

func (m *ChangeNotification) Serialize() ([]byte, error) {
    // Simple binary encoding (could use protobuf/msgpack in production)
    // Format: [key_len][key][version][checksum_len][checksum][timestamp][node_id_len][node_id]

    buf := make([]byte, 0, 256)

    // Key
    keyLen := uint16(len(m.Key))
    buf = binary.BigEndian.AppendUint16(buf, keyLen)
    buf = append(buf, []byte(m.Key)...)

    // Version
    buf = binary.BigEndian.AppendUint64(buf, uint64(m.Version))

    // Checksum
    checksumLen := uint16(len(m.Checksum))
    buf = binary.BigEndian.AppendUint16(buf, checksumLen)
    buf = append(buf, []byte(m.Checksum)...)

    // Timestamp
    buf = binary.BigEndian.AppendUint64(buf, uint64(m.Timestamp.Unix()))

    // NodeID
    nodeIDLen := uint16(len(m.NodeID))
    buf = binary.BigEndian.AppendUint16(buf, nodeIDLen)
    buf = append(buf, []byte(m.NodeID)...)

    return buf, nil
}

func DeserializeChangeNotification(data []byte) (*ChangeNotification, error) {
    if len(data) < 2 {
        return nil, fmt.Errorf("data too short")
    }

    msg := &ChangeNotification{}
    offset := 0

    // Key
    keyLen := binary.BigEndian.Uint16(data[offset:])
    offset += 2
    msg.Key = string(data[offset : offset+int(keyLen)])
    offset += int(keyLen)

    // Version
    msg.Version = int64(binary.BigEndian.Uint64(data[offset:]))
    offset += 8

    // Checksum
    checksumLen := binary.BigEndian.Uint16(data[offset:])
    offset += 2
    msg.Checksum = string(data[offset : offset+int(checksumLen)])
    offset += int(checksumLen)

    // Timestamp
    timestamp := int64(binary.BigEndian.Uint64(data[offset:]))
    msg.Timestamp = time.Unix(timestamp, 0)
    offset += 8

    // NodeID
    nodeIDLen := binary.BigEndian.Uint16(data[offset:])
    offset += 2
    msg.NodeID = string(data[offset : offset+int(nodeIDLen)])

    return msg, nil
}

// JoinRequest is sent by new nodes joining the cluster
type JoinRequest struct {
    NodeID    string
    Address   string
    Timestamp time.Time
}

func (m *JoinRequest) Type() MessageType {
    return MsgJoinRequest
}

// JoinAck is the response to a join request
type JoinAck struct {
    ClusterID string
    Peers     []PeerInfo
}

func (m *JoinAck) Type() MessageType {
    return MsgJoinAck
}

type PeerInfo struct {
    NodeID   string
    Address  string
    LastSeen time.Time
}
```

### Step 4: Network Layer (Day 6-8)

**SOLID**: Single Responsibility - Transport handles only network I/O

#### 4.1 TCP/UDP Transport

```go
// internal/network/transport.go
package network

import (
    "context"
    "encoding/binary"
    "fmt"
    "net"
    "sync"

    "github.com/sanketn26/gossipcache/internal/gossip"
)

const (
    MagicNumber = 0x47534350 // "GSCP"
    Version     = 1
)

// Transport handles network communication
// SRP: Responsible only for sending/receiving messages
type Transport struct {
    tcpListener net.Listener
    udpConn     *net.UDPConn

    handlers map[gossip.MessageType]MessageHandler
    mu       sync.RWMutex

    closed   chan struct{}
    wg       sync.WaitGroup
}

type MessageHandler func(msg gossip.Message, from net.Addr) error

func NewTransport(tcpPort, udpPort int) (*Transport, error) {
    // TCP listener
    tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", tcpPort))
    if err != nil {
        return nil, fmt.Errorf("tcp listen: %w", err)
    }

    // UDP connection
    udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", udpPort))
    if err != nil {
        tcpListener.Close()
        return nil, fmt.Errorf("resolve udp addr: %w", err)
    }

    udpConn, err := net.ListenUDP("udp", udpAddr)
    if err != nil {
        tcpListener.Close()
        return nil, fmt.Errorf("udp listen: %w", err)
    }

    t := &Transport{
        tcpListener: tcpListener,
        udpConn:     udpConn,
        handlers:    make(map[gossip.MessageType]MessageHandler),
        closed:      make(chan struct{}),
    }

    // Start listeners
    t.wg.Add(2)
    go t.listenTCP()
    go t.listenUDP()

    return t, nil
}

func (t *Transport) RegisterHandler(msgType gossip.MessageType, handler MessageHandler) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.handlers[msgType] = handler
}

func (t *Transport) SendTCP(ctx context.Context, addr string, msg gossip.Message) error {
    conn, err := net.Dial("tcp", addr)
    if err != nil {
        return err
    }
    defer conn.Close()

    return t.writeMessage(conn, msg)
}

func (t *Transport) SendUDP(ctx context.Context, addr string, msg gossip.Message) error {
    udpAddr, err := net.ResolveUDPAddr("udp", addr)
    if err != nil {
        return err
    }

    data, err := t.encodeMessage(msg)
    if err != nil {
        return err
    }

    _, err = t.udpConn.WriteToUDP(data, udpAddr)
    return err
}

func (t *Transport) listenTCP() {
    defer t.wg.Done()

    for {
        select {
        case <-t.closed:
            return
        default:
        }

        conn, err := t.tcpListener.Accept()
        if err != nil {
            continue
        }

        go t.handleTCPConn(conn)
    }
}

func (t *Transport) handleTCPConn(conn net.Conn) {
    defer conn.Close()

    msg, err := t.readMessage(conn)
    if err != nil {
        return
    }

    t.dispatchMessage(msg, conn.RemoteAddr())
}

func (t *Transport) listenUDP() {
    defer t.wg.Done()

    buf := make([]byte, 65536)

    for {
        select {
        case <-t.closed:
            return
        default:
        }

        n, addr, err := t.udpConn.ReadFromUDP(buf)
        if err != nil {
            continue
        }

        msg, err := t.decodeMessage(buf[:n])
        if err != nil {
            continue
        }

        t.dispatchMessage(msg, addr)
    }
}

func (t *Transport) dispatchMessage(msg gossip.Message, from net.Addr) {
    t.mu.RLock()
    handler, ok := t.handlers[msg.Type()]
    t.mu.RUnlock()

    if ok && handler != nil {
        handler(msg, from)
    }
}

func (t *Transport) writeMessage(conn net.Conn, msg gossip.Message) error {
    data, err := t.encodeMessage(msg)
    if err != nil {
        return err
    }

    _, err = conn.Write(data)
    return err
}

func (t *Transport) readMessage(conn net.Conn) (gossip.Message, error) {
    // Read header (magic + version + type + length)
    header := make([]byte, 12)
    if _, err := conn.Read(header); err != nil {
        return nil, err
    }

    // Verify magic number
    magic := binary.BigEndian.Uint32(header[0:4])
    if magic != MagicNumber {
        return nil, fmt.Errorf("invalid magic number")
    }

    // Read message type and length
    msgType := gossip.MessageType(binary.BigEndian.Uint16(header[6:8]))
    length := binary.BigEndian.Uint32(header[8:12])

    // Read payload
    payload := make([]byte, length)
    if _, err := conn.Read(payload); err != nil {
        return nil, err
    }

    return t.deserializeMessage(msgType, payload)
}

func (t *Transport) encodeMessage(msg gossip.Message) ([]byte, error) {
    payload, err := msg.Serialize()
    if err != nil {
        return nil, err
    }

    // Build message: [magic][version][type][length][payload]
    buf := make([]byte, 12+len(payload))

    binary.BigEndian.PutUint32(buf[0:4], MagicNumber)
    binary.BigEndian.PutUint16(buf[4:6], Version)
    binary.BigEndian.PutUint16(buf[6:8], uint16(msg.Type()))
    binary.BigEndian.PutUint32(buf[8:12], uint32(len(payload)))
    copy(buf[12:], payload)

    return buf, nil
}

func (t *Transport) decodeMessage(data []byte) (gossip.Message, error) {
    if len(data) < 12 {
        return nil, fmt.Errorf("message too short")
    }

    msgType := gossip.MessageType(binary.BigEndian.Uint16(data[6:8]))
    payload := data[12:]

    return t.deserializeMessage(msgType, payload)
}

func (t *Transport) deserializeMessage(msgType gossip.MessageType, payload []byte) (gossip.Message, error) {
    switch msgType {
    case gossip.MsgChangeNotification:
        return gossip.DeserializeChangeNotification(payload)
    // Add other message types...
    default:
        return nil, fmt.Errorf("unknown message type: %d", msgType)
    }
}

func (t *Transport) Close() error {
    close(t.closed)

    t.tcpListener.Close()
    t.udpConn.Close()

    t.wg.Wait()
    return nil
}
```

### Step 4.5: Connection Pooling & Backpressure (Day 8-9)

**SOLID**: Single Responsibility - Connection manager handles only connection lifecycle

#### 4.5.1 TCP Connection Pool

```go
// internal/network/pool.go
package network

import (
    "context"
    "errors"
    "net"
    "sync"
    "time"
)

// ConnectionPool manages TCP connections to peers
type ConnectionPool struct {
    pools map[string]*peerPool
    mu    sync.RWMutex

    maxConnsPerPeer int
    connTimeout     time.Duration
}

type peerPool struct {
    addr  string
    conns chan net.Conn
    mu    sync.Mutex
}

func NewConnectionPool(maxConnsPerPeer int, connTimeout time.Duration) *ConnectionPool {
    return &ConnectionPool{
        pools:           make(map[string]*peerPool),
        maxConnsPerPeer: maxConnsPerPeer,
        connTimeout:     connTimeout,
    }
}

func (cp *ConnectionPool) Get(ctx context.Context, addr string) (net.Conn, error) {
    pool := cp.getOrCreatePool(addr)

    // Try to get existing connection
    select {
    case conn := <-pool.conns:
        // Check if connection is still alive
        if err := checkConn(conn); err == nil {
            return conn, nil
        }
        conn.Close()
    default:
    }

    // Create new connection
    dialer := &net.Dialer{Timeout: cp.connTimeout}
    return dialer.DialContext(ctx, "tcp", addr)
}

func (cp *ConnectionPool) Put(addr string, conn net.Conn) {
    pool := cp.getOrCreatePool(addr)

    select {
    case pool.conns <- conn:
        // Connection returned to pool
    default:
        // Pool full, close connection
        conn.Close()
    }
}

func (cp *ConnectionPool) getOrCreatePool(addr string) *peerPool {
    cp.mu.RLock()
    pool, exists := cp.pools[addr]
    cp.mu.RUnlock()

    if exists {
        return pool
    }

    cp.mu.Lock()
    defer cp.mu.Unlock()

    // Double check
    if pool, exists = cp.pools[addr]; exists {
        return pool
    }

    pool = &peerPool{
        addr:  addr,
        conns: make(chan net.Conn, cp.maxConnsPerPeer),
    }
    cp.pools[addr] = pool

    return pool
}

func checkConn(conn net.Conn) error {
    // Set short deadline for check
    conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
    defer conn.SetReadDeadline(time.Time{})

    one := make([]byte, 1)
    _, err := conn.Read(one)

    if err == nil {
        // Put byte back (shouldn't happen for healthy conn)
        return errors.New("unexpected data")
    }

    // Check for timeout (connection is alive)
    if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
        return nil
    }

    return err
}

func (cp *ConnectionPool) Close() error {
    cp.mu.Lock()
    defer cp.mu.Unlock()

    for _, pool := range cp.pools {
        close(pool.conns)
        for conn := range pool.conns {
            conn.Close()
        }
    }

    return nil
}
```

#### 4.5.2 Update Transport with Connection Pool

```go
// internal/network/transport.go (updated)
func NewTransport(tcpPort, udpPort int) (*Transport, error) {
    // ... existing code ...

    t := &Transport{
        tcpListener: tcpListener,
        udpConn:     udpConn,
        handlers:    make(map[gossip.MessageType]MessageHandler),
        connPool:    NewConnectionPool(10, 5*time.Second), // Add pool
        closed:      make(chan struct{}),
    }

    return t, nil
}

func (t *Transport) SendTCP(ctx context.Context, addr string, msg gossip.Message) error {
    // Get connection from pool
    conn, err := t.connPool.Get(ctx, addr)
    if err != nil {
        return err
    }

    // Try to send message
    err = t.writeMessage(conn, msg)

    if err != nil {
        // Connection failed, close it
        conn.Close()
        return err
    }

    // Return connection to pool
    t.connPool.Put(addr, conn)
    return nil
}
```

#### 4.5.3 Backpressure Handling

```go
// internal/gossip/queue.go
package gossip

import (
    "context"
    "errors"
    "sync"
)

var ErrQueueFull = errors.New("gossip queue full")

// GossipQueue provides backpressure for gossip messages
type GossipQueue struct {
    messages chan *QueuedMessage
    maxSize  int
    dropped  int64 // atomic counter
    mu       sync.RWMutex
}

type QueuedMessage struct {
    Message gossip.Message
    Target  string
    Retries int
}

func NewGossipQueue(maxSize int) *GossipQueue {
    return &GossipQueue{
        messages: make(chan *QueuedMessage, maxSize),
        maxSize:  maxSize,
    }
}

func (gq *GossipQueue) Enqueue(ctx context.Context, msg *QueuedMessage) error {
    select {
    case gq.messages <- msg:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Queue full - apply backpressure
        atomic.AddInt64(&gq.dropped, 1)
        return ErrQueueFull
    }
}

func (gq *GossipQueue) Dequeue(ctx context.Context) (*QueuedMessage, error) {
    select {
    case msg := <-gq.messages:
        return msg, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (gq *GossipQueue) Len() int {
    return len(gq.messages)
}

func (gq *GossipQueue) Dropped() int64 {
    return atomic.LoadInt64(&gq.dropped)
}
```

#### 4.5.4 Integrate Queue with Gossip Engine

```go
// internal/gossip/engine.go (updated)
type Engine struct {
    nodeID  string

    transport   *network.Transport
    storage     storage.Storage
    backingStore backingstore.BackingStore
    peers       *PeerManager

    queue       *GossipQueue  // Add queue
    config      *Config
    closed      chan struct{}
    wg          sync.WaitGroup
}

func NewEngine(...) *Engine {
    e := &Engine{
        // ... existing fields ...
        queue: NewGossipQueue(1000), // Add queue
    }

    // Start queue processor
    e.wg.Add(1)
    go e.processQueue()

    return e
}

func (e *Engine) BroadcastChange(ctx context.Context, key string, version int64, value []byte) error {
    checksum := fmt.Sprintf("%x", sha256.Sum256(value))

    msg := &ChangeNotification{
        Key:       key,
        Version:   version,
        Checksum:  checksum,
        Timestamp: time.Now(),
        NodeID:    e.nodeID,
    }

    // Enqueue messages for async processing
    peers := e.selectGossipPeers()

    for _, peer := range peers {
        queuedMsg := &QueuedMessage{
            Message: msg,
            Target:  peer.Address,
            Retries: 0,
        }

        // Non-blocking enqueue with backpressure
        if err := e.queue.Enqueue(ctx, queuedMsg); err != nil {
            // Log dropped message
            logger.Warn("gossip queue full, message dropped",
                "peer", peer.Address,
                "dropped_total", e.queue.Dropped())
        }
    }

    return nil
}

func (e *Engine) processQueue() {
    defer e.wg.Done()

    for {
        select {
        case <-e.closed:
            return
        default:
        }

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        msg, err := e.queue.Dequeue(ctx)
        cancel()

        if err != nil {
            continue
        }

        // Send message
        if err := e.transport.SendUDP(context.Background(), msg.Target, msg.Message); err != nil {
            // Retry logic
            if msg.Retries < 3 {
                msg.Retries++
                e.queue.Enqueue(context.Background(), msg)
            }
        }
    }
}
```

#### 4.5.5 Testing Connection Pool and Backpressure

```go
// internal/network/pool_test.go
func TestConnectionPool_ReuseConnection(t *testing.T) {
    pool := NewConnectionPool(5, 5*time.Second)
    defer pool.Close()

    // Start test server
    listener := startTestServer(t)
    defer listener.Close()

    ctx := context.Background()

    // Get connection
    conn1, err := pool.Get(ctx, listener.Addr().String())
    require.NoError(t, err)

    // Return to pool
    pool.Put(listener.Addr().String(), conn1)

    // Get again - should reuse
    conn2, err := pool.Get(ctx, listener.Addr().String())
    require.NoError(t, err)

    // Should be same connection (can verify with conn.LocalAddr())
    assert.Equal(t, conn1.LocalAddr(), conn2.LocalAddr())
}

// internal/gossip/queue_test.go
func TestGossipQueue_Backpressure(t *testing.T) {
    queue := NewGossipQueue(10) // Small queue

    ctx := context.Background()

    // Fill queue
    for i := 0; i < 10; i++ {
        msg := &QueuedMessage{Target: fmt.Sprintf("peer%d", i)}
        err := queue.Enqueue(ctx, msg)
        require.NoError(t, err)
    }

    // Next message should trigger backpressure
    msg := &QueuedMessage{Target: "peer11"}
    err := queue.Enqueue(ctx, msg)
    assert.ErrorIs(t, err, ErrQueueFull)
    assert.Equal(t, int64(1), queue.Dropped())
}
```

**Benefits**:
- **Connection Pooling**: Reduces connection overhead, improves throughput
- **Backpressure**: Prevents memory exhaustion, graceful degradation
- **Observability**: Track dropped messages, queue depth

### Step 5: Gossip Engine (Day 9-13)

**SOLID**: Single Responsibility - Gossip engine coordinates gossip protocol

#### 5.1 Peer Management

```go
// internal/gossip/peer.go
package gossip

import (
    "sync"
    "time"
)

// PeerManager tracks cluster peers
type PeerManager struct {
    mu    sync.RWMutex
    peers map[string]*Peer
}

type Peer struct {
    NodeID   string
    Address  string
    LastSeen time.Time
    Status   PeerStatus
}

type PeerStatus int

const (
    StatusAlive PeerStatus = iota
    StatusSuspected
    StatusDead
)

func NewPeerManager() *PeerManager {
    return &PeerManager{
        peers: make(map[string]*Peer),
    }
}

func (pm *PeerManager) AddPeer(nodeID, address string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    pm.peers[nodeID] = &Peer{
        NodeID:   nodeID,
        Address:  address,
        LastSeen: time.Now(),
        Status:   StatusAlive,
    }
}

func (pm *PeerManager) GetPeers() []*Peer {
    pm.mu.RLock()
    defer pm.mu.RUnlock()

    peers := make([]*Peer, 0, len(pm.peers))
    for _, peer := range pm.peers {
        peers = append(peers, peer)
    }

    return peers
}

func (pm *PeerManager) UpdateLastSeen(nodeID string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    if peer, ok := pm.peers[nodeID]; ok {
        peer.LastSeen = time.Now()
        peer.Status = StatusAlive
    }
}

func (pm *PeerManager) GetAlivePeers() []*Peer {
    pm.mu.RLock()
    defer pm.mu.RUnlock()

    peers := make([]*Peer, 0)
    for _, peer := range pm.peers {
        if peer.Status == StatusAlive {
            peers = append(peers, peer)
        }
    }

    return peers
}
```

#### 5.2 Gossip Engine Implementation

```go
// internal/gossip/engine.go
package gossip

import (
    "context"
    "crypto/sha256"
    "fmt"
    "math/rand"
    "sync"
    "time"

    "github.com/sanketn26/gossipcache/internal/backingstore"
    "github.com/sanketn26/gossipcache/internal/network"
    "github.com/sanketn26/gossipcache/internal/storage"
)

// Engine implements the gossip protocol
// SRP: Coordinates gossip rounds, change propagation, anti-entropy
type Engine struct {
    nodeID  string

    transport   *network.Transport
    storage     storage.Storage
    backingStore backingstore.BackingStore
    peers       *PeerManager

    config      *Config
    closed      chan struct{}
    wg          sync.WaitGroup
}

type Config struct {
    Interval            time.Duration
    Fanout              int
    AntiEntropyInterval time.Duration
}

func NewEngine(
    nodeID string,
    transport *network.Transport,
    storage storage.Storage,
    backingStore backingstore.BackingStore,
    config *Config,
) *Engine {
    e := &Engine{
        nodeID:       nodeID,
        transport:    transport,
        storage:      storage,
        backingStore: backingStore,
        peers:        NewPeerManager(),
        config:       config,
        closed:       make(chan struct{}),
    }

    // Register message handlers
    transport.RegisterHandler(MsgChangeNotification, e.handleChangeNotification)
    transport.RegisterHandler(MsgJoinRequest, e.handleJoinRequest)

    return e
}

func (e *Engine) Start(ctx context.Context) error {
    // Start gossip loop
    e.wg.Add(1)
    go e.gossipLoop()

    // Start anti-entropy loop
    e.wg.Add(1)
    go e.antiEntropyLoop()

    return nil
}

func (e *Engine) Stop() error {
    close(e.closed)
    e.wg.Wait()
    return nil
}

func (e *Engine) AddPeer(nodeID, address string) {
    e.peers.AddPeer(nodeID, address)
}

// BroadcastChange sends change notification to peers (backed mode)
func (e *Engine) BroadcastChange(ctx context.Context, key string, version int64, value []byte) error {
    // Calculate checksum
    checksum := fmt.Sprintf("%x", sha256.Sum256(value))

    msg := &ChangeNotification{
        Key:       key,
        Version:   version,
        Checksum:  checksum,
        Timestamp: time.Now(),
        NodeID:    e.nodeID,
    }

    // Select random peers
    peers := e.selectGossipPeers()

    // Send to each peer (async, best-effort)
    for _, peer := range peers {
        go func(p *Peer) {
            ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
            defer cancel()

            e.transport.SendUDP(ctx, p.Address, msg)
        }(peer)
    }

    return nil
}

func (e *Engine) handleChangeNotification(msg Message, from net.Addr) error {
    notification, ok := msg.(*ChangeNotification)
    if !ok {
        return fmt.Errorf("invalid message type")
    }

    ctx := context.Background()

    // Get local entry
    localEntry, err := e.storage.Get(ctx, notification.Key)

    // If not in local cache or version is newer, pull from backing store
    if err != nil || localEntry == nil || e.needsPull(localEntry, notification) {
        return e.pullFromBackingStore(ctx, notification)
    }

    return nil
}

func (e *Engine) needsPull(localEntry *storage.Entry, notification *ChangeNotification) bool {
    // For backed mode, we could store version in metadata
    // For simplicity, always pull if checksum differs
    localChecksum := fmt.Sprintf("%x", sha256.Sum256(localEntry.Value))
    return localChecksum != notification.Checksum
}

func (e *Engine) pullFromBackingStore(ctx context.Context, notification *ChangeNotification) error {
    // Pull from backing store
    value, version, err := e.backingStore.Get(ctx, notification.Key)
    if err != nil {
        return fmt.Errorf("pull from backing store: %w", err)
    }

    // Verify version
    if version < notification.Version {
        // Backing store is stale, this shouldn't happen
        return fmt.Errorf("backing store version mismatch")
    }

    // Update local cache
    // We need to extend storage to support version metadata
    // For now, use TTL-based approach
    return e.storage.Set(ctx, notification.Key, value, 5*time.Minute)
}

func (e *Engine) selectGossipPeers() []*Peer {
    alivePeers := e.peers.GetAlivePeers()

    if len(alivePeers) <= e.config.Fanout {
        return alivePeers
    }

    // Select random subset
    rand.Shuffle(len(alivePeers), func(i, j int) {
        alivePeers[i], alivePeers[j] = alivePeers[j], alivePeers[i]
    })

    return alivePeers[:e.config.Fanout]
}

func (e *Engine) gossipLoop() {
    defer e.wg.Done()

    ticker := time.NewTicker(e.config.Interval)
    defer ticker.Stop()

    for {
        select {
        case <-e.closed:
            return
        case <-ticker.C:
            // Gossip round happens on-demand when changes occur
            // This loop can be used for heartbeats/failure detection
        }
    }
}

func (e *Engine) antiEntropyLoop() {
    defer e.wg.Done()

    ticker := time.NewTicker(e.config.AntiEntropyInterval)
    defer ticker.Stop()

    for {
        select {
        case <-e.closed:
            return
        case <-ticker.C:
            e.performAntiEntropy()
        }
    }
}

func (e *Engine) performAntiEntropy() {
    peers := e.selectGossipPeers()
    if len(peers) == 0 {
        return
    }

    // Select random peer for anti-entropy
    peer := peers[rand.Intn(len(peers))]

    // Anti-entropy implementation (simplified)
    // In production: exchange merkle trees, identify differences, sync
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    tree, err := e.buildMerkleTree(ctx)
    if err != nil {
        logger.Warn("failed to build merkle tree", "error", err)
        return
    }

    req := &AntiEntropyRequest{
        NodeID:     e.nodeID,
        MerkleRoot: tree.RootHash(),
        KeyCount:   tree.KeyCount(),
        RequestID:  e.newRequestID(),
    }

    if err := e.transport.SendTCP(ctx, peer.Address, req); err != nil {
        logger.Warn("anti-entropy request failed", "peer", peer.NodeID, "error", err)
    }
}

func (e *Engine) handleJoinRequest(msg Message, from net.Addr) error {
    // Handle node joining cluster
    joinReq, ok := msg.(*JoinRequest)
    if !ok {
        return fmt.Errorf("invalid message type")
    }

    // Add to peers
    e.peers.AddPeer(joinReq.NodeID, joinReq.Address)

    ack := &JoinAck{
        ClusterID:    e.config.ClusterID,
        Peers:        e.peers.GetAliveNodeInfo(),
        GossipConfig: e.config.PublicGossipConfig(),
    }

    return e.transport.SendTCP(context.Background(), joinReq.Address, ack)
}
```

#### 5.5 Merkle Tree Comparison

```go
// internal/gossip/merkle.go
package gossip

import (
    "crypto/sha256"
    "fmt"
    "sort"
)

type MerkleTree struct {
    leaves []MerkleLeaf
    root   []byte
}

type MerkleLeaf struct {
    Key     string
    Version int64
    Hash    []byte
}

func BuildMerkleTree(entries []MerkleLeaf) *MerkleTree {
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Key < entries[j].Key
    })

    hashes := make([][]byte, 0, len(entries))
    for _, entry := range entries {
        sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", entry.Key, entry.Version)))
        hashes = append(hashes, sum[:])
    }

    return &MerkleTree{
        leaves: entries,
        root:   buildMerkleRoot(hashes),
    }
}

func (m *MerkleTree) RootHash() []byte {
    return append([]byte(nil), m.root...)
}

func (m *MerkleTree) KeyCount() int {
    return len(m.leaves)
}
```

Anti-entropy handling:
- If Merkle roots match, return success without transferring keys.
- If roots differ, exchange subtree hashes to narrow the differing key ranges.
- Pull differing keys from the backing store in backed mode, then update local storage.
- Track anti-entropy request latency, differing-key count, and repair errors in metrics.

### Step 6: Integrate with Cache Manager (Day 14-16)

**SOLID**: Dependency Inversion - Cache manager depends on abstractions

```go
// internal/cache/backed_cache.go
package cache

import (
    "context"
    "time"

    "github.com/sanketn26/gossipcache/internal/backingstore"
    "github.com/sanketn26/gossipcache/internal/gossip"
    "github.com/sanketn26/gossipcache/internal/storage"
    "github.com/sanketn26/gossipcache/internal/util"
)

// BackedCache implements Cache with backing store support
type BackedCache struct {
    storage      storage.Storage
    backingStore backingstore.BackingStore
    gossip       *gossip.Engine
    sf           *util.SingleFlight // DRY: Reuse singleflight
    config       *CacheConfig
}

func NewBackedCache(
    storage storage.Storage,
    backingStore backingstore.BackingStore,
    gossip *gossip.Engine,
    config *CacheConfig,
) *BackedCache {
    return &BackedCache{
        storage:      storage,
        backingStore: backingStore,
        gossip:       gossip,
        sf:           util.NewSingleFlight(),
        config:       config,
    }
}

func (c *BackedCache) Get(ctx context.Context, key string) ([]byte, error) {
    // Try local cache first
    entry, err := c.storage.Get(ctx, key)
    if err == nil {
        return entry.Value, nil
    }

    // Cache miss: pull from backing store (with singleflight)
    result := c.sf.Do(key, func() (interface{}, error) {
        return c.pullFromBackingStore(ctx, key)
    })

    if result.Err != nil {
        return nil, result.Err
    }

    return result.Val.([]byte), nil
}

func (c *BackedCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    // Write to backing store
    version, err := c.backingStore.Set(ctx, key, value)
    if err != nil {
        return err
    }

    // Update local cache
    if err := c.storage.Set(ctx, key, value, ttl); err != nil {
        return err
    }

    // Gossip change notification (async)
    go c.gossip.BroadcastChange(context.Background(), key, version, value)

    return nil
}

func (c *BackedCache) pullFromBackingStore(ctx context.Context, key string) ([]byte, error) {
    value, _, err := c.backingStore.Get(ctx, key)
    if err != nil {
        return nil, err
    }

    // Update local cache
    c.storage.Set(ctx, key, value, c.config.DefaultTTL)

    return value, nil
}
```

### Step 7: Integration Tests (Day 17-20)

```go
// test/integration/backed_mode_test.go
package integration

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

func TestBackedMode_MultiNode(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Setup 3-node cluster
    nodes := setupThreeNodeCluster(t)
    defer cleanupCluster(nodes)

    ctx := context.Background()

    // Write on node 1
    err := nodes[0].Set(ctx, "test_key", []byte("test_value"), 5*time.Minute)
    require.NoError(t, err)

    // Wait for gossip propagation
    time.Sleep(2 * time.Second)

    // Read from node 2 (should pull from Redis)
    value, err := nodes[1].Get(ctx, "test_key")
    require.NoError(t, err)
    require.Equal(t, []byte("test_value"), value)

    // Read from node 3 (should hit local cache now)
    value, err = nodes[2].Get(ctx, "test_key")
    require.NoError(t, err)
    require.Equal(t, []byte("test_value"), value)
}
```

## Deliverables

- [ ] Backing store interface with TTL semantics and `ExpiresAt`-aware reads
- [ ] Redis implementation with atomic version+TTL via Lua (covers Valkey)
- [ ] Memcached implementation with `cas`-based versioning and exptime quirk handling
- [ ] Gossip protocol with metadata propagation
- [ ] Network layer (TCP/UDP)
- [ ] Peer management and discovery
- [ ] Change detection and pull mechanism
- [ ] Singleflight pattern implemented
- [ ] Multi-node integration tests
- [ ] Performance benchmarks

## TDD Test Plan

| Slice | Write This Test First | Expected Behavior | Checkpoint |
| --- | --- | --- | --- |
| Backing store contract | `internal/backingstore/backingstore_test.go` | Store interface supports get/set/delete/multi-key/version/TTL semantics through a fake | `go test ./internal/backingstore` |
| Redis connector | `internal/backingstore/redis/redis_test.go` | Set increments version atomically, Get returns value/version, missing maps to not found | `go test ./internal/backingstore/redis` |
| Redis integration | `internal/backingstore/redis/redis_integration_test.go` | Real Redis round-trips values and survives reconnects | `go test -tags=integration ./internal/backingstore/redis` |
| Redis TTL | `internal/backingstore/redis/redis_ttl_test.go` | Set with TTL surfaces `ExpiresAt`, key expires in Redis after TTL, Set with `ttl=0` clears any existing TTL, negative TTL is rejected | `go test -tags=integration ./internal/backingstore/redis` |
| Memcached connector | `internal/backingstore/memcached/memcached_test.go` | Framed version round-trips via Get, Set produces monotonic versions, CAS conflict triggers retry, missing key returns `ErrKeyNotFound` | `go test ./internal/backingstore/memcached` |
| Memcached integration | `internal/backingstore/memcached/memcached_integration_test.go` | Real memcached round-trips values, concurrent writers see monotonic versions, delete is idempotent | `go test -tags=integration ./internal/backingstore/memcached` |
| Memcached TTL | `internal/backingstore/memcached/memcached_ttl_test.go` | `encodeExptime` produces relative seconds for TTL ≤ 30 days and absolute timestamp above, sub-second TTL rounds up to 1s, key expires after TTL, negative TTL is rejected | `go test -tags=integration ./internal/backingstore/memcached` |
| Gossip messages | `internal/gossip/message_test.go` | Each message reports the correct type and validates required fields | `go test ./internal/gossip` |
| Codec | `internal/network/codec_test.go` | Encode/decode round-trips messages and rejects bad magic, version, and truncated frames | `go test ./internal/network` |
| Transport | `internal/network/transport_test.go` | Peer send/receive uses context cancellation and returns useful errors | `go test -race ./internal/network` |
| Connection pool | `internal/network/pool_test.go` | Pool reuses connections and respects max connection limits | `go test -race ./internal/network` |
| Backpressure | `internal/gossip/queue_test.go` | Queue drops, blocks, or rejects according to configured policy under pressure | `go test -race ./internal/gossip` |
| Gossip engine | `internal/gossip/engine_test.go` | Change notifications fan out to selected peers and ignore stale/self-originated messages | `go test -race ./internal/gossip` |
| Pull mechanism | `internal/cache/backed_cache_test.go` | Cache miss pulls from backing store once with singleflight and stores locally | `go test -race ./internal/cache ./internal/backingstore/...` |
| Multi-node backed mode | `test/integration/backed_mode_test.go` | Three nodes converge through metadata gossip and Redis-backed pulls | `go test -tags=integration ./test/integration` |

## Success Criteria

1. **Functional**: 3-node cluster with backed mode working
2. **Performance**: Gossip propagation < 500ms
3. **Quality**: >80% test coverage, integration tests passing
4. **Scalability**: Tested with 5-10 nodes

## Next Phase

Once Phase 2 is complete, move to [Phase 3: Independent Mode](PHASE_3_INDEPENDENT_MODE.md) to add vector clocks and full-data gossip.

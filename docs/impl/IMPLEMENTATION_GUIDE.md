# GossipCache Implementation Guide

This guide is the hands-on path through the implementation. Use it beside the phase docs when you want to sit down, write code, run tests, and make the next small design decision.

The goal is not to copy every snippet exactly. The goal is to implement each layer in a way that keeps the public API stable, isolates complexity, and gives you a working checkpoint after every few files.

## How To Use This Guide

Work in thin vertical slices:

1. Define the interface.
2. Add the smallest implementation that compiles.
3. Write tests for the behavior.
4. Wire it into the next layer.
5. Run `go test ./...`.
6. Pause and answer the design questions before moving on.

Reference docs:
- [Package Structure](PACKAGE_STRUCTURE.md)
- [Phase 1 Foundation](PHASE_1_FOUNDATION.md)
- [Phase 2 Backed Mode](PHASE_2_BACKED_MODE.md)
- [Phase 3 Independent Mode](PHASE_3_INDEPENDENT_MODE.md)
- [Phase 4 Production](PHASE_4_PRODUCTION.md)
- [Testing Strategy](TESTING_STRATEGY.md)

## Implementation Order

Implement in this order so each step builds on something testable:

1. Public API and errors
2. Config and validation
3. Storage interface and memory storage
4. Cache manager for single-node local mode
5. Backing store interface and Redis
6. Gossip message model and codec
7. Network transport, connection pool, and backpressure queue
8. Backed-mode cache with metadata gossip
9. Vector clocks and conflict resolvers
10. Independent-mode full-data gossip
11. Anti-entropy and Merkle tree repair
12. Discovery providers
13. HTTP API, metrics, health, debug, and pprof
14. Deployment manifests and runbooks
15. Optional security hardening

## Step 1: Public API

Start with the contract that users will import. Keep this package small and boring.

Create:
- `pkg/gossipcache/cache.go`
- `pkg/gossipcache/types.go`
- `pkg/gossipcache/errors.go`
- `pkg/gossipcache/client.go`

Example:

```go
// pkg/gossipcache/cache.go
package gossipcache

import (
    "context"
    "time"
)

type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)
    SetMulti(ctx context.Context, entries map[string][]byte, ttl time.Duration) error
    Flush(ctx context.Context) error
    Stats(ctx context.Context) (*Stats, error)
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

```go
// pkg/gossipcache/types.go
package gossipcache

import "time"

type Mode string

const (
    ModeBacked      Mode = "backed"
    ModeIndependent Mode = "independent"
)

type Config struct {
    Mode       Mode
    NodeID     string
    Address    string
    Cache      CacheConfig
    Gossip     GossipConfig
    Backing    BackingStoreConfig
    Discovery  DiscoveryConfig
    HTTP       HTTPConfig
}

type CacheConfig struct {
    MaxSizeBytes   int64
    DefaultTTL     time.Duration
    EvictionPolicy string
}

type Stats struct {
    Hits      int64
    Misses    int64
    Evictions int64
    SizeBytes int64
    Keys      int64
    PeerCount int
}
```

Design questions:
- Which fields must be public API forever?
- Which fields are implementation detail and should stay in `internal/config`?
- What defaults should make a one-node cache usable with almost no config?

Checkpoint:

```bash
go test ./pkg/gossipcache
```

## Step 2: Internal Config

The public config can be user-friendly. The internal config should be validated and complete.

Create:
- `internal/config/config.go`
- `internal/config/defaults.go`
- `internal/config/validator.go`

Example:

```go
// internal/config/validator.go
package config

import (
    "errors"
    "time"
)

func Validate(cfg *Config) error {
    if cfg.NodeID == "" {
        return errors.New("node_id is required")
    }
    if cfg.Cache.MaxSizeBytes <= 0 {
        return errors.New("cache.max_size_bytes must be positive")
    }
    if cfg.Cache.DefaultTTL < 0 {
        return errors.New("cache.default_ttl cannot be negative")
    }
    if cfg.Gossip.Interval <= 0 {
        cfg.Gossip.Interval = time.Second
    }
    if cfg.Gossip.Fanout <= 0 {
        cfg.Gossip.Fanout = 3
    }
    return nil
}
```

Implementation notes:
- Keep validation deterministic.
- Do not let lower layers guess missing required fields.
- Convert public config to internal config once, near startup.

## Step 3: Storage Interface

Storage is the core of both modes. Get this clean before touching gossip.

Create:
- `internal/storage/storage.go`
- `internal/storage/entry.go`
- `internal/storage/memory/memory.go`
- `internal/storage/memory/lru.go`

Example:

```go
// internal/storage/storage.go
package storage

import (
    "context"
    "errors"
    "time"
)

var ErrKeyNotFound = errors.New("key not found")

type Storage interface {
    Get(ctx context.Context, key string) (*Entry, error)
    Set(ctx context.Context, entry *Entry) error
    Delete(ctx context.Context, key string) error
    GetMulti(ctx context.Context, keys []string) (map[string]*Entry, error)
    Flush(ctx context.Context) error
    Stats(ctx context.Context) (*Stats, error)
    Close() error
}

type Entry struct {
    Key       string
    Value     []byte
    Version   int64
    Checksum  string
    VectorClock map[string]int64
    TTL       time.Duration
    ExpiresAt time.Time
    CreatedAt time.Time
    UpdatedAt time.Time
    Tombstone bool
}

func (e *Entry) Expired(now time.Time) bool {
    return !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt)
}
```

Example memory shape:

```go
// internal/storage/memory/memory.go
package memory

import (
    "context"
    "sync"
    "time"

    "github.com/sanketn26/gossipcache/internal/storage"
)

type Store struct {
    mu      sync.RWMutex
    entries map[string]*storage.Entry
    maxSize int64
    now     func() time.Time
}

func New(maxSize int64) *Store {
    return &Store{
        entries: make(map[string]*storage.Entry),
        maxSize: maxSize,
        now:     time.Now,
    }
}

func (s *Store) Get(ctx context.Context, key string) (*storage.Entry, error) {
    s.mu.RLock()
    entry, ok := s.entries[key]
    s.mu.RUnlock()
    if !ok || entry.Expired(s.now()) {
        return nil, storage.ErrKeyNotFound
    }

    copy := *entry
    copy.Value = append([]byte(nil), entry.Value...)
    return &copy, nil
}

func (s *Store) Set(ctx context.Context, entry *storage.Entry) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    copy := *entry
    copy.Value = append([]byte(nil), entry.Value...)
    if copy.CreatedAt.IsZero() {
        copy.CreatedAt = s.now()
    }
    copy.UpdatedAt = s.now()
    s.entries[copy.Key] = &copy
    return nil
}
```

Tests to write:
- Set then get returns a copy.
- Missing key returns `ErrKeyNotFound`.
- Expired key behaves like missing.
- Delete removes a key.
- Concurrent get/set does not race.

Checkpoint:

```bash
go test -race ./internal/storage/...
```

Design questions:
- Should expired entries be removed on read, in a janitor, or both?
- Should storage own checksums, or should callers provide them?
- What metadata is common enough to belong in `storage.Entry`?

## Step 4: Single-Node Cache Manager

Now wrap storage with the public `Cache` behavior.

Create:
- `internal/cache/cache.go`
- `internal/cache/local_cache.go`
- `internal/cache/stats.go`

Example:

```go
// internal/cache/local_cache.go
package cache

import (
    "context"
    "crypto/sha256"
    "fmt"
    "sync/atomic"
    "time"

    "github.com/sanketn26/gossipcache/internal/storage"
)

type LocalCache struct {
    store      storage.Storage
    defaultTTL time.Duration
    hits       atomic.Int64
    misses     atomic.Int64
}

func NewLocal(store storage.Storage, defaultTTL time.Duration) *LocalCache {
    return &LocalCache{store: store, defaultTTL: defaultTTL}
}

func (c *LocalCache) Get(ctx context.Context, key string) ([]byte, error) {
    entry, err := c.store.Get(ctx, key)
    if err != nil {
        c.misses.Add(1)
        return nil, err
    }
    c.hits.Add(1)
    return append([]byte(nil), entry.Value...), nil
}

func (c *LocalCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    if ttl == 0 {
        ttl = c.defaultTTL
    }

    var expiresAt time.Time
    if ttl > 0 {
        expiresAt = time.Now().Add(ttl)
    }

    sum := sha256.Sum256(value)
    return c.store.Set(ctx, &storage.Entry{
        Key:       key,
        Value:     value,
        Checksum:  fmt.Sprintf("%x", sum),
        TTL:       ttl,
        ExpiresAt: expiresAt,
    })
}
```

Checkpoint:

```bash
go test ./internal/cache ./internal/storage/...
```

Design questions:
- Should `Set` accept empty keys?
- Should `GetMulti` return partial results when some keys are missing?
- What errors should cross the public API boundary?

## Step 5: Backing Store Interface

Before Redis, define the abstraction.

Create:
- `internal/backingstore/backingstore.go`
- `internal/backingstore/errors.go`

Example:

```go
// internal/backingstore/backingstore.go
package backingstore

import (
    "context"
    "time"
)

type Store interface {
    Get(ctx context.Context, key string) ([]byte, int64, error)
    Set(ctx context.Context, key string, value []byte) (int64, error)
    Delete(ctx context.Context, key string) error
    GetMulti(ctx context.Context, keys []string) (map[string]*Entry, error)
    SetMulti(ctx context.Context, entries map[string][]byte) (map[string]int64, error)
    Ping(ctx context.Context) error
    Close() error
}

type Entry struct {
    Key     string
    Value   []byte
    Version int64
}

type Config struct {
    Type     string
    Address  string
    Database string
    Username string
    Password string
    PoolSize int
    Timeout  time.Duration
}
```

Redis implementation thinking:
- Store value and version together.
- Increment version atomically.
- Treat Valkey as Redis-compatible.
- Keep Redis-specific errors inside the Redis package.

Example Redis `Set`:

```go
func (r *Store) Set(ctx context.Context, key string, value []byte) (int64, error) {
    script := redis.NewScript(`
        local key = KEYS[1]
        local version = redis.call("HGET", key, "version")
        if not version then
            version = 0
        end
        version = tonumber(version) + 1
        redis.call("HSET", key, "value", ARGV[1], "version", version)
        return version
    `)

    version, err := script.Run(ctx, r.client, []string{"cache:" + key}, value).Int64()
    if err != nil {
        return 0, err
    }
    return version, nil
}
```

Checkpoint:

```bash
go test ./internal/backingstore/...
```

## Step 6: Gossip Messages And Codec

Build the message model before the network transport.

Create:
- `internal/gossip/message.go`
- `internal/network/codec.go`

Example:

```go
// internal/gossip/message.go
package gossip

import "time"

type MessageType uint16

const (
    MsgChangeNotification MessageType = 1
    MsgDataUpdate         MessageType = 2
    MsgAntiEntropyRequest MessageType = 3
    MsgAntiEntropyReply   MessageType = 4
    MsgJoinRequest        MessageType = 5
    MsgJoinAck            MessageType = 6
)

type Message interface {
    Type() MessageType
}

type ChangeNotification struct {
    Key       string
    Version   int64
    Checksum  string
    Timestamp time.Time
    NodeID    string
}

func (ChangeNotification) Type() MessageType {
    return MsgChangeNotification
}
```

Codec shape:

```go
// internal/network/codec.go
package network

import (
    "encoding/binary"
    "encoding/json"
    "fmt"
    "io"
)

const magic uint32 = 0x47534350
const protocolVersion uint16 = 1

func WriteFrame(w io.Writer, msgType uint16, payload []byte) error {
    header := make([]byte, 12)
    binary.BigEndian.PutUint32(header[0:4], magic)
    binary.BigEndian.PutUint16(header[4:6], protocolVersion)
    binary.BigEndian.PutUint16(header[6:8], msgType)
    binary.BigEndian.PutUint32(header[8:12], uint32(len(payload)))

    if _, err := w.Write(header); err != nil {
        return err
    }
    _, err := w.Write(payload)
    return err
}

func DecodePayload[T any](payload []byte) (*T, error) {
    var out T
    if err := json.Unmarshal(payload, &out); err != nil {
        return nil, fmt.Errorf("decode payload: %w", err)
    }
    return &out, nil
}
```

Design questions:
- Is JSON acceptable for MVP debugging, or do you want MessagePack/Protobuf now?
- Where will compression plug in later without changing callers?
- How will unknown message types be handled?

## Step 7: Network Transport And Backpressure

Create:
- `internal/network/transport.go`
- `internal/network/pool.go`
- `internal/gossip/queue.go`

Keep network transport responsible for bytes and connections. Keep gossip responsible for message semantics.

Example queue:

```go
// internal/gossip/queue.go
package gossip

import (
    "context"
    "errors"
    "sync/atomic"
)

var ErrQueueFull = errors.New("gossip queue full")

type Queue struct {
    messages chan QueuedMessage
    dropped  atomic.Int64
}

type QueuedMessage struct {
    Target  string
    Message Message
    Retries int
}

func NewQueue(size int) *Queue {
    return &Queue{messages: make(chan QueuedMessage, size)}
}

func (q *Queue) Enqueue(ctx context.Context, msg QueuedMessage) error {
    select {
    case q.messages <- msg:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    default:
        q.dropped.Add(1)
        return ErrQueueFull
    }
}
```

Checkpoint:

```bash
go test -race ./internal/network ./internal/gossip
```

## Step 8: Backed Mode

Backed mode writes through to the backing store and gossips metadata.

Create:
- `internal/cache/backed_cache.go`
- `internal/gossip/backed_engine.go`

Write flow:

```go
func (c *BackedCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    version, err := c.backing.Set(ctx, key, value)
    if err != nil {
        return err
    }

    if err := c.local.Set(ctx, key, value, ttl); err != nil {
        return err
    }

    return c.gossip.BroadcastChange(ctx, key, version, value)
}
```

Read flow:

```go
func (c *BackedCache) Get(ctx context.Context, key string) ([]byte, error) {
    value, err := c.local.Get(ctx, key)
    if err == nil {
        return value, nil
    }

    value, version, err := c.backing.Get(ctx, key)
    if err != nil {
        return nil, err
    }

    _ = c.store.Set(ctx, &storage.Entry{
        Key:     key,
        Value:   value,
        Version: version,
        TTL:     c.defaultTTL,
    })
    return append([]byte(nil), value...), nil
}
```

Implementation questions:
- When backing store is down, do reads serve stale values?
- Do writes fail closed, or queue for retry?
- How do you expose stale reads in HTTP responses?

## Step 9: Vector Clocks

Create:
- `internal/vclock/vclock.go`
- `internal/vclock/compare.go`

Example:

```go
package vclock

type Clock map[string]int64

type Relation int

const (
    Equal Relation = iota
    LocalNewer
    RemoteNewer
    Concurrent
)

func Compare(local, remote Clock) Relation {
    localGreater := false
    remoteGreater := false

    seen := make(map[string]struct{}, len(local)+len(remote))
    for node := range local {
        seen[node] = struct{}{}
    }
    for node := range remote {
        seen[node] = struct{}{}
    }

    for node := range seen {
        if local[node] > remote[node] {
            localGreater = true
        }
        if remote[node] > local[node] {
            remoteGreater = true
        }
    }

    switch {
    case localGreater && remoteGreater:
        return Concurrent
    case localGreater:
        return LocalNewer
    case remoteGreater:
        return RemoteNewer
    default:
        return Equal
    }
}
```

Tests:
- equal clocks
- local newer
- remote newer
- concurrent clocks
- missing node IDs count as zero

## Step 10: Conflict Resolvers

Create:
- `internal/conflict/resolver.go`
- `internal/conflict/lww.go`
- `internal/conflict/siblings.go`

Example:

```go
package conflict

import "github.com/sanketn26/gossipcache/internal/storage"

type Resolver interface {
    Resolve(local, remote *storage.Entry) (*storage.Entry, error)
}

type LastWriteWins struct{}

func (LastWriteWins) Resolve(local, remote *storage.Entry) (*storage.Entry, error) {
    if remote.UpdatedAt.After(local.UpdatedAt) {
        return remote, nil
    }
    return local, nil
}
```

Design questions:
- Is wall-clock LWW acceptable for your target use case?
- Should siblings be exposed to callers or only HTTP/admin APIs?
- How long do tombstones live?

## Step 11: Independent Mode

Independent mode gossips full values and uses vector clocks.

Create:
- `internal/cache/independent_cache.go`
- `internal/gossip/independent_engine.go`

Write flow:

```go
func (c *IndependentCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    clock := c.nextClock(key)

    entry := &storage.Entry{
        Key:         key,
        Value:       value,
        VectorClock: clock,
        TTL:         ttl,
        UpdatedAt:   time.Now(),
    }

    if err := c.store.Set(ctx, entry); err != nil {
        return err
    }
    return c.gossip.BroadcastUpdate(ctx, entry)
}
```

Merge flow:

```go
func (e *Engine) ApplyRemoteUpdate(ctx context.Context, remote *storage.Entry) error {
    local, err := e.store.Get(ctx, remote.Key)
    if errors.Is(err, storage.ErrKeyNotFound) {
        return e.store.Set(ctx, remote)
    }
    if err != nil {
        return err
    }

    switch vclock.Compare(local.VectorClock, remote.VectorClock) {
    case vclock.RemoteNewer:
        return e.store.Set(ctx, remote)
    case vclock.Concurrent:
        resolved, err := e.resolver.Resolve(local, remote)
        if err != nil {
            return err
        }
        return e.store.Set(ctx, resolved)
    default:
        return nil
    }
}
```

Checkpoint:

```bash
go test ./internal/vclock ./internal/conflict ./internal/cache
```

## Step 12: Anti-Entropy

Anti-entropy repairs missed gossip.

Create:
- `internal/gossip/merkle.go`
- anti-entropy request handlers in the gossip engine

Process:

1. Build a Merkle tree from `key + version` in backed mode.
2. Build a Merkle tree from `key + vector_clock_hash` in independent mode.
3. Exchange roots.
4. If roots differ, narrow by subtree.
5. Repair differing keys.

Example leaf:

```go
type MerkleLeaf struct {
    Key      string
    StateTag string
}

func leafHash(leaf MerkleLeaf) []byte {
    sum := sha256.Sum256([]byte(leaf.Key + ":" + leaf.StateTag))
    return sum[:]
}
```

Implementation questions:
- How many keys can be compared per anti-entropy round?
- Should large repairs be rate limited?
- Which metrics show repair health?

## Step 13: Discovery

Create:
- `internal/discovery/discovery.go`
- `internal/discovery/static_discovery.go`
- `internal/discovery/dns_discovery.go`
- provider-specific files for EC2, Docker, Kubernetes

Interface:

```go
package discovery

import "context"

type Provider interface {
    Discover(ctx context.Context) ([]NodeInfo, error)
    Register(ctx context.Context, self NodeInfo) error
    Deregister(ctx context.Context, nodeID string) error
    Watch(ctx context.Context) (<-chan []NodeInfo, error)
}

type NodeInfo struct {
    NodeID  string
    Address string
}
```

Start with static discovery because it is deterministic and easy to test.

```go
type Static struct {
    peers []NodeInfo
}

func (s *Static) Discover(ctx context.Context) ([]NodeInfo, error) {
    out := make([]NodeInfo, len(s.peers))
    copy(out, s.peers)
    return out, nil
}
```

Checkpoint:

```bash
go test ./internal/discovery/...
```

## Step 14: HTTP API And Observability

Create:
- `internal/api/server.go`
- `internal/api/handlers.go`
- `internal/api/middleware.go`
- `internal/observability/metrics.go`
- `internal/observability/health.go`

API handler shape:

```go
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
    key := strings.TrimPrefix(r.URL.Path, "/api/v1/cache/")
    value, err := s.cache.Get(r.Context(), key)
    if errors.Is(err, storage.ErrKeyNotFound) {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write(value)
}
```

Metrics to add early:
- cache hits and misses
- set/delete counts
- gossip sent/received counts
- gossip queue depth and drops
- peer count by status
- backing store latency/errors

Debug endpoint rule:
- Disabled by default.
- Never expose cache samples without explicit config.
- pprof should bind to a trusted interface or require auth in hardened deployments.

## Step 15: Command Wiring

Create:
- `cmd/gossipcache/main.go`

Startup order:

1. Load config.
2. Validate config.
3. Create logger and metrics.
4. Create storage.
5. Create backing store if needed.
6. Create transport.
7. Create discovery provider.
8. Create gossip engine.
9. Create cache.
10. Start gossip, discovery watch, HTTP server.
11. Wait for shutdown signal.
12. Stop components in reverse order.

Example:

```go
func run(ctx context.Context, cfg *config.Config) error {
    store := memory.New(cfg.Cache.MaxSizeBytes)
    transport, err := network.NewTransport(cfg.Network)
    if err != nil {
        return err
    }
    defer transport.Close()

    engine := gossip.NewEngine(cfg.Gossip, transport, store)
    cache := cache.NewLocal(store, cfg.Cache.DefaultTTL)

    if err := engine.Start(ctx); err != nil {
        return err
    }
    defer engine.Stop(context.Background())

    return waitForSignal(ctx)
}
```

## Step 16: Testing Routine

Use this loop for every slice:

```bash
go test ./...
go test -race ./internal/...
go test -bench=. ./test/benchmark/...
```

Before a phase is complete:
- Unit tests pass.
- Race detector passes for touched packages.
- Public API examples compile.
- Integration tests pass for any external dependency introduced.
- Benchmarks do not regress unexpectedly.

## Step 17: Production Pass

Before calling the MVP done:

1. Run a three-node backed-mode cluster with Redis.
2. Run a three-node independent-mode cluster.
3. Kill and restart one node.
4. Simulate a network partition.
5. Verify convergence after healing.
6. Run anti-entropy manually.
7. Confirm metrics and health endpoints.
8. Confirm debug endpoints are disabled by default.
9. Load test reads and writes.
10. Document the operational behavior you observed.

## Design Journal Prompts

Keep a short design journal as you implement. Useful prompts:

- What invariant does this package own?
- What can fail here, and who handles that failure?
- Is this package hiding an external dependency behind an interface?
- Does this API force callers to know too much?
- What test would catch the most expensive bug in this layer?
- What metric would help debug this in production?
- What is intentionally deferred?

## Suggested First Milestone

Aim for this before touching Redis or gossip:

- `pkg/gossipcache` public API compiles.
- `internal/config` validates defaults.
- `internal/storage/memory` supports get/set/delete/ttl.
- `internal/cache` supports single-node get/set/delete/stats.
- `cmd/gossipcache` can start a local cache and shut down cleanly.
- `go test ./...` passes.

That gives you a real foundation. From there, backed mode and independent mode become feature additions instead of a pile of intertwined first attempts.

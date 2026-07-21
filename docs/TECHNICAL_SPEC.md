# Technical Specification (sketch)

**Normative behavior:** [SEMANTICS.md](SEMANTICS.md)
**Wire/state-machine detail:** [impl/HYBRID_BACKED_MODE.md](impl/HYBRID_BACKED_MODE.md)

This file is a thin **Go-facing sketch** only—not a second source of product rules.

## Runtime

- Go 1.21+
- L1 library in-process; L2 hub process(es)
- mTLS TCP for L2 RPC and invalidation streams

## Core types

```go
// Version tag — partition_id is part of the tag (SEMANTICS §3).
type VersionTag struct {
    PartitionID uint32
    Sequence    uint64
}

type KeyState int
const (
    StateEmpty KeyState = iota
    StateFetching
    StateValid
    StateStale
)

type StalePolicy int
const (
    StaleNever StalePolicy = iota
    StaleIfError
    ServeStaleWhileRevalidate
)

// W = 0 default (SEMANTICS §8).
type WriteOptions struct {
    W            int           // peer confirms before OK; 0 = async return
    ConfirmLevel ConfirmLevel  // InvalidateApplied in v1
    Timeout      time.Duration // when W > 0
}

type ConfirmLevel int
const (
    ConfirmInvalidateApplied ConfirmLevel = iota
    // ConfirmValueVisible // later
)
```

## Cache API

```go
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    SetWithOptions(ctx context.Context, key string, value []byte, ttl time.Duration, opt WriteOptions) error
    Delete(ctx context.Context, key string) error
    DeleteWithOptions(ctx context.Context, key string, opt WriteOptions) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

## L2 client

```go
type L2Client interface {
    Get(ctx context.Context, key string, min *VersionTag) (val []byte, ver VersionTag, ttl time.Duration, err error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) (VersionTag, error)
    Delete(ctx context.Context, key string) (VersionTag, error)
    SubscribeInvalidations(ctx context.Context, partitions []uint32) (<-chan InvalidationBatch, error)
    Close() error
}
```

## Config (illustrative)

```go
type Config struct {
    NodeID              string
    L2Addresses         []string
    StalePolicy         StalePolicy
    DefaultWriteW       int           // 0
    StreamFreshnessTimeout time.Duration
    TCPPortRPC          int           // e.g. 7400 toward hub
    TCPPortControl      int           // e.g. 7401 streams
    MgmtListen          string        // e.g. 127.0.0.1:8081
    MetricsListen       string        // e.g. :9090
    TLS                 TLSConfig
}
```

## Health

| Path | Role |
|------|------|
| `/livez` | Liveness |
| `/startupz` | Startup complete |
| `/readyz` | Consistency-safe to serve (SEMANTICS §10) |

## Out of scope here

Protobuf/binary layouts, full state transition tables, metrics catalog → [impl/HYBRID_BACKED_MODE.md](impl/HYBRID_BACKED_MODE.md).

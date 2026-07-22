# Technical Specification (sketch)

**Normative behavior:** [SEMANTICS.md](SEMANTICS.md)
**Implementation contracts:** [impl/README.md](impl/README.md)

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

// Sketch only; the single frozen definition is Common P0.
type WriteOptions struct {
    W            uint16        // peer confirms before OK; 0 = async return
    Mode         WriteMode     // durability ack; WriteFast default
    ConfirmLevel ConfirmLevel  // InvalidateApplied in v1
    Timeout      time.Duration // when W > 0
}

type ConfirmLevel uint8
const (
    ConfirmInvalidateApplied ConfirmLevel = iota
    // ConfirmValueVisible // later
)

// Durability acknowledgement, independent of W (SEMANTICS §7).
type WriteMode uint8
const (
    WriteFast WriteMode = iota // ack on atomic hub memory commit (default)
    WriteSync                  // ack after synchronous durability fence; needs durable hub
)
// WriteSync on a memory-only or unhealthy backend returns ErrDurabilityUnavailable.

// Hub storage profile, fixed for a hub lifetime and advertised at handshake.
type StorageProfile uint8
const (
    StorageMemory  StorageProfile = iota // ephemeral; new hub_generation each start (default)
    StorageDurable                       // opt-in synchronous persistence + recovery
)
```

## Public Node API

```go
type Client struct { /* unexported fields */ }
func New(cfg Config) (*Client, error)
func (c *Client) Get(ctx context.Context, key string) ([]byte, bool, error)
func (c *Client) Set(ctx context.Context, key string, value []byte, ttl time.Duration, opts ...WriteOption) error
func (c *Client) Delete(ctx context.Context, key string, opts ...WriteOption) error
func (c *Client) Start(ctx context.Context) error
func (c *Client) Stop(ctx context.Context) error
```

## Internal Hub seam

This consumer-owned interface belongs to `internal/l1`; it is not exported API.
Control-stream consumption is a separate `internal/control` responsibility.

```go
type HubClient interface {
    Get(ctx context.Context, key []byte, min *VersionTag) (GetResult, error)
    Set(ctx context.Context, key, value []byte, ttl time.Duration, opt WriteOptions) (VersionTag, error)
    Delete(ctx context.Context, key []byte, opt WriteOptions) (VersionTag, error)
}
```

`GetResult` includes value/tombstone kind, version, TTL, status and
`HubGeneration`, as frozen by Common P1/P3.

## Config (illustrative)

```go
type Config struct {
    NodeID              string
    L2Addresses         []string
    StalePolicy         StalePolicy
    DefaultWriteW       uint16        // 0
    DefaultWriteMode    WriteMode     // WriteFast
    // Hub storage profile is advertised by the handshake, not selected by Node.
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

The authoritative API/type definitions, wire layouts, transitions and metrics
are owned by their [phase files](impl/README.md); this sketch must follow them.

# Node P0 — Foundation

**Common contract:** [COMMON_PHASE_00_CONTRACTS.md](../common/COMMON_PHASE_00_CONTRACTS.md).

## Functional work

- [ ] Land the public `Client` facade and idempotent lifecycle surface.
- [ ] Define the consumer-owned hub client interface.
- [ ] Reuse current local memory storage behind the emerging node boundary.
- [ ] Wire the in-memory hub fake without minting versions in the node.
- [ ] Preserve context and mutable-byte copy semantics.

## Implementation detail

### Public facade (`pkg/gossipcache`)

```go
type Client struct { /* unexported */ }

func New(cfg Config) (*Client, error)
func (c *Client) Get(ctx context.Context, key string) ([]byte, bool, error)
func (c *Client) Set(ctx context.Context, key string, val []byte, ttl time.Duration, opts ...WriteOption) error
func (c *Client) Delete(ctx context.Context, key string, opts ...WriteOption) error
func (c *Client) Start(ctx context.Context) error // idempotent
func (c *Client) Stop(ctx context.Context) error  // idempotent, joins goroutines
```

`WriteOption` is a functional-option shim over the Common P0 `wire.WriteOptions`
(`WithW(k)`, `WithSync()`, `WithTimeout(d)`); default is `WriteFast`, `W=0`.

### Hub seam (consumer-owned)

The node depends on the `HubClient` interface (common P1); P0 wires the
in-memory fake so the node never mints versions. The interface is injected
through `Config` so P3 can swap in the real RPC client with no node-side change.

```go
type Config struct {
    Hub              HubClient    // injected; fake in P0, RPC client in P3
    StalePolicy      StalePolicy
    DefaultWriteW    uint16       // 0; matches wire.WriteOptions.W
    DefaultWriteMode WriteMode    // WriteFast; matches wire.WriteOptions.Mode
    // ... freshness timeout, listeners added in later phases
}
```

### Reuse of existing storage

The current `internal/storage/memory` sharded map is reused behind the emerging
`internal/l1` slot boundary for local retention; context propagation and
`wire.CopyBytes` boundary copies are preserved. No global authority lives in the
node.

## Verification

- [ ] Public facade/default configuration tests.
- [ ] One node performs miss, Set, Get, Delete and TTL through the fake hub.
- [ ] Closed-client and cancellation behavior tests.

**Exit:** the application-facing node works through an injected authoritative
hub seam; local hits remain in process.

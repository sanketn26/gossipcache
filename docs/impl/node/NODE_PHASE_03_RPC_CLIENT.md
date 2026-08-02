# Node P3 — Real hub gRPC client

**Depends on:** [NODE_PHASE_02_STREAM_CONSUMER.md](NODE_PHASE_02_STREAM_CONSUMER.md).

**Common contract:** [COMMON_PHASE_03_DATA_PROTOCOL.md](../common/COMMON_PHASE_03_DATA_PROTOCOL.md).

## Functional work

- [ ] Implement bounded dial, request, retry and cancellation against gRPC `Data`.
- [ ] Carry one stable request ID across mutation retries.
- [ ] Map application statuses to public sentinel errors.
- [ ] Preserve/install a committed version returned with `W` timeout.
- [ ] Validate response partition and `hub_generation`.
- [ ] Record the Hub storage profile from handshake/status without changing the
  cache-consistency algorithm.
- [ ] Send `WriteFast` by default and expose explicit `WriteSync` through write
  options.
- [ ] Map unsupported/unhealthy Sync to `ErrDurabilityUnavailable` and never
  install a mutation that the Hub rejected before commit.
- [ ] Add jittered retry/circuit behavior without retrying terminal failures.

## Implementation detail

### Real client (`internal/rpc`, node side)

Implements the same `HubClient` interface the fake satisfied, so swapping it in
changes no node semantics. Uses generated `gossipcachev1.DataClient`.

```go
type grpcClient struct {
    conn    *grpc.ClientConn
    data    gossipcachev1.DataClient
    profile atomic.Uint32 // StorageProfile from handshake
    healthy atomic.Bool   // DurableHealthy from handshake/status
}
```

Unary RPCs carry the caller's `context` for deadlines/cancellation. There is no
hand-rolled correlation-id map; gRPC owns request multiplexing.

### Request lifecycle

- **Dial/retry:** bounded dial timeout; retryable application statuses
  (`NOT_CAUGHT_UP`, `ERR_RATE_LIMITED`) and transport reset retry with jittered
  backoff and a circuit breaker; terminal statuses
  (`ERR_DURABILITY_UNAVAILABLE`, `ERR_BAD_GENERATION`) never retry.
- **Idempotent retries:** the same `MutationID` is reused across retries so the
  hub dedup cache collapses duplicates — a timeout+retry cannot double-apply.
- **Status → sentinel mapping:** application statuses map to public errors
  (`NOT_CAUGHT_UP` → `ErrNotCaughtUp`, `ERR_DURABILITY_UNAVAILABLE` →
  `ErrDurabilityUnavailable`, `ERR_WRITE_CONFIRM_TIMEOUT` →
  `ErrWriteConfirmTimeout`, `ERR_BAD_GENERATION` → `ErrBadGeneration`). A W
  timeout response still carries the committed `VersionTag`, which the Node
  installs before returning the error.
- **Validation:** every response's `PartitionID` and `hub_generation` are checked;
  a generation mismatch surfaces `ErrBadGeneration` and triggers revalidation.
- **Conversion:** `internal/rpc` Proto* / FromProto* helpers at the boundary.

### Durability intent

- Default `WriteFast`; `WithSync()` sets `WriteSync`. The client records the
  advertised `StorageProfile`/`DurableHealthy` from the handshake but **never
  changes the cache-consistency algorithm** based on it.
- If Sync is requested against a memory/unhealthy hub, the hub rejects before
  commit; the client maps it to `ErrDurabilityUnavailable` and installs nothing.

## Verification

- [ ] Run the same client contract suite against fake and real hubs.
- [ ] Timeout/retry does not duplicate a mutation.
- [ ] `NOT_CAUGHT_UP`, tombstone, TTL and generation mismatch tests.
- [ ] Memory-Hub restart forces old-generation entries out of ready service.
- [ ] Fast/Sync × W result matrix distinguishes durability errors from peer
  confirmation timeouts.

**Exit:** replacing the fake with a real hub changes no node semantics.

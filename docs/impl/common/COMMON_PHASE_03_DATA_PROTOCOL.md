# Common P3 — Data RPC (gRPC) and storage-profile contract

**Depends on:** [COMMON_PHASE_02_CONTROL_PROTOCOL.md](COMMON_PHASE_02_CONTROL_PROTOCOL.md).

## Transport and schema

Data is the **gRPC** service `gossipcache.v1.Data` on port **7400** (shared with
Control):

| RPC | Role |
|-----|------|
| `Handshake` | Protocol negotiation; `cluster_id` + expected generation; hub advertisement |
| `HubStatus` | Active storage posture (requires `hub_generation`) |
| `Get` | Authoritative read (requires `hub_generation` **before** lookup) |
| `Mutate` | Set/Delete (requires `hub_generation` **before** commit) |

| Artifact | Location |
|----------|----------|
| IDL | [`api/proto/gossipcache/v1/data.proto`](../../../api/proto/gossipcache/v1/data.proto) (+ `common.proto`) |
| Generated stubs | `api/gen/gossipcache/v1` |
| Domain helpers | `internal/rpc` |

Regenerate: `make proto`.

Application-level result codes use the protobuf / `wire.Status` enum on
responses. gRPC status codes are for transport, auth, and cancellation.

### Generation and cluster (per call)

Unary RPCs are not a session. After bootstrap Handshake adopts
`hub_generation`, the Node **must** send that generation on every Get/Mutate
and HubStatus. The Hub compares for equality **before** lookup or any commit /
sequence assignment. Mismatch returns `STATUS_ERR_BAD_GENERATION` and
**commits nothing**.

`HandshakeRequest` / `Hello` carry `cluster_id` and `expected_hub_generation`
(0 = bootstrap). See [COMMON_PHASE_06_SECURITY.md](COMMON_PHASE_06_SECURITY.md).

## RPC rules

- [x] Freeze transport/schema as gRPC + protobuf.
- [x] Carry `cluster_id` and expected generation on Handshake; `hub_generation`
  on Get/Mutate/HubStatus.
- [x] Carry bounded keys/values, TTL, complete version, status and mutation ID.
- [x] Define retryable/terminal statuses and cancellation behavior.
- [x] Scope request deduplication by authenticated Node and retention window.
- [x] Preserve committed version in a W-timeout response.
- [x] Define `min_version` / `NOT_CAUGHT_UP` behavior.
- [x] Carry active `memory` or `durable` Hub storage profile in handshake/status.
- [x] Carry `WriteFast` (default) or `WriteSync` on every mutation.
- [x] Define `ErrDurabilityUnavailable` and committed-result error details.

## Profile rules

Both profiles atomically commit value/tombstone, expiry, `VersionTag`, stream
sequence, dedup result and changefeed event. TTL expiry uses the same mutation
path as Delete.

- `memory` is default. Success means committed to Hub memory; restart loses
  state and creates a different `hub_generation`.
- `durable` is opt-in and supports both acknowledgement modes.
- `WriteFast` succeeds after atomic memory commit.
- `WriteSync` requires healthy durable storage and fences prior partition
  mutations.
- A W-timeout never changes the profile commit result.
- Generation mismatch never commits.

## Implementation detail

### Handshake and requests (domain shape)

```go
type HandshakeRequest struct {
    Protocol              wire.ProtocolRange
    ClusterID             string
    ExpectedHubGeneration uint64 // 0 = bootstrap
}
type Handshake struct {
    ProtocolVersion wire.ProtocolVersion
    HubGeneration   uint64
    PartitionCount  uint32
    StorageProfile  wire.StorageProfile
    DurableHealthy  bool
    ClusterID       string
}
type GetRequest struct {
    Key           []byte
    MinVersion    *wire.VersionTag
    HubGeneration uint64 // required; checked before lookup
}
type MutationRequest struct {
    Op, Key, Value, TTLMillis, MutationID, Mode, W, Confirm, Timeout
    HubGeneration uint64 // required; checked before commit
}
```

### Status classification

| Status | Class | Meaning |
|--------|-------|---------|
| `OK` | success | committed per selected Mode |
| `NOT_FOUND` | success | no live value; no version |
| `NOT_CAUGHT_UP` | retryable | `min_version` above committed head |
| `ERR_RATE_LIMITED`, transport reset | retryable | retry with same `MutationID` |
| `ERR_DURABILITY_UNAVAILABLE` | terminal | Sync on memory/unhealthy hub; nothing committed |
| `ERR_BAD_GENERATION` | terminal | generation mismatch; nothing committed / no value served |
| `ERR_INVALID_ARGUMENT` | terminal | malformed request or mismatched `MutationID` reuse |
| `ERR_WRITE_CONFIRM_TIMEOUT` | success+ | commit succeeded; W peers not confirmed; carries version |

### Deduplication

- Hub keeps a per-partition dedup entry keyed by `MutationID` for
  `DedupWindow` (default 5 min).
- Dedup is scoped by authenticated Node identity.
- Generation is checked **before** dedup lookup/commit so a stale generation
  cannot join a waiter or observe a prior commit under the wrong generation.

## Cross-component verification

- [ ] Fake and real Hub pass the same Node client contract suite.
- [x] Domain validation and protobuf round-trips in `internal/rpc`.
- [ ] Generation mismatch on Mutate commits nothing (unit + integration).
- [ ] Timeout/retry cannot duplicate a mutation.
- [ ] Memory restart loses state safely through generation change.

**Common exit (schema):** gRPC Data service, per-RPC generation, status classes,
profile advertisement, and mutation fingerprint rules frozen via protobuf +
domain helpers.

**Full phase exit:** requires Hub/Node P3 runtime.

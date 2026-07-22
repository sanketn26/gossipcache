# Common P3 — Data RPC and storage-profile contract

**Depends on:** [COMMON_PHASE_02_CONTROL_PROTOCOL.md](COMMON_PHASE_02_CONTROL_PROTOCOL.md).

## RPC rules

- [ ] Select and freeze RPC transport/schema.
- [ ] Carry bounded keys/values, TTL, complete version, `hub_generation`, status
  and mutation request ID.
- [ ] Define retryable/terminal statuses and cancellation behavior.
- [ ] Scope request deduplication by authenticated Node and retention window.
- [ ] Preserve committed version in a W-timeout response.
- [ ] Define `min_version` / `NOT_CAUGHT_UP` behavior.
- [ ] Carry the active `memory` or `durable` Hub storage profile in handshake
  and management status.
- [ ] Carry `WriteFast` (default) or `WriteSync` on every mutation.
- [ ] Define `ErrDurabilityUnavailable` and committed-result error details.

## Profile rules

Both profiles atomically commit value/tombstone, expiry, `VersionTag`, stream
sequence, dedup result and changefeed event. TTL expiry uses the same mutation
path as Delete.

- `memory` is default. Success means committed to Hub memory; restart loses
  state and creates a different `hub_generation`.
- `durable` is opt-in and supports both acknowledgement modes.
- `WriteFast` succeeds after atomic memory commit. Ordered persistence may run
  asynchronously, but restart survival is not promised.
- `WriteSync` requires healthy durable storage and succeeds only after this
  mutation and all earlier partition mutations are synchronously persisted.
- Unsupported/unhealthy Sync fails without committing the mutation.
- A W-timeout never changes the profile commit result. In memory mode it does
  not imply restart durability.
- Nodes use the same API in both profiles and must not infer durability beyond
  the advertised profile.

## Implementation detail

### Transport and RPC surface (`internal/rpc`)

- Transport: length-prefixed request/response frames over the same mTLS TCP
  substrate as control (separate port `7400`), one in-flight table keyed by a
  4-byte `correlation_id`. gRPC is explicitly **not** used in v1 to keep the
  wire frozen and dependency-free.
- Handshake advertises hub identity and durability posture:

```go
type Handshake struct {
    ProtocolVersion uint16
    HubGeneration   uint64
    PartitionCount  uint32
    StorageProfile  wire.StorageProfile // memory | durable, fixed for hub lifetime
    DurableHealthy  bool                // durable profile only; gates WriteSync
}
```

- Requests carry the full tag context and durability intent:

```go
type MutationRequest struct {
    Op         Op            // Set | Delete
    Key, Value []byte
    TTLMillis  uint64
    MutationID wire.MutationID
    Mode       wire.WriteMode // Fast (default) | Sync
    W          uint16         // peer confirms; orthogonal to Mode
    Confirm    wire.ConfirmLevel // InvalidateApplied is the only v1 value
    Timeout    uint32         // ms, when W > 0
}
type MutationResponse struct {
    Status        wire.Status
    HubGeneration uint64
    Version       wire.VersionTag // present even on ErrWriteConfirmTimeout
}
type GetRequest struct { Key []byte; MinVersion *wire.VersionTag }
type GetResponse struct {
    Status        wire.Status // OK | NOT_FOUND | NOT_CAUGHT_UP | error
    HubGeneration uint64
    Version       wire.VersionTag
    Value         []byte
    TTLMillis     uint64
    Kind          wire.RecordKind
}
```

The Hub stamps every response with the generation under which it evaluated the
request. The Node compares it with the adopted connection generation before
using a value or installing a committed mutation. A mismatch fails closed and
starts the Common P6 generation-change path.

### Data-plane encoding and stored records

RPC frames and durable records are deliberately separate encodings; the deleted
legacy `0xC7` value envelope is not a v1 compatibility format.

- The selected RPC schema encodes every request/response field above, uses a
  length-prefixed frame with format version and CRC32C, and has golden vectors
  under `internal/rpc/testdata/` for Get/Set/Delete, tombstone,
  `NOT_CAUGHT_UP`, W timeout and every terminal status.
- The durable WAL record is the checksummed layout owned by Hub P3. It includes
  partition, version, stream sequence, record kind, key, value, expiry and
  dedup/request metadata; recovery golden vectors live under
  `internal/l2/durable/testdata/`.
- Neither side persists or transmits an opaque implementation-specific Go
  struct. Unknown format versions fail closed.

### Status classification (frozen here)

| Status | Class | Meaning |
|--------|-------|---------|
| `OK` | success | committed per selected Mode |
| `NOT_FOUND` | success | no live value; no version |
| `NOT_CAUGHT_UP` | retryable | `min_version` above committed head |
| `ERR_RATE_LIMITED`, transport reset | retryable | retry with same `MutationID` |
| `ERR_DURABILITY_UNAVAILABLE` | terminal | Sync on memory/unhealthy hub; nothing committed |
| `ERR_BAD_GENERATION` | terminal | reconnect/revalidate |
| `ERR_INVALID_ARGUMENT` | terminal | malformed request or mismatched reuse of a `MutationID`; original mutation unchanged |
| `ERR_WRITE_CONFIRM_TIMEOUT` | success+ | commit succeeded; W peers not confirmed in time; response carries committed version |

The Node maps wire `ERR_WRITE_CONFIRM_TIMEOUT` to public
`ErrWriteConfirmTimeout`. It installs the response's committed version before
returning the error; the status is never treated as a retryable commit failure.

### Deduplication and idempotency

- The hub keeps a per-partition dedup entry keyed by `MutationID` for
  `DedupWindow` (default 5 min). It records the immutable request fingerprint,
  committed `VersionTag`, and the original W waiter/final outcome. A retry
  within the window commits nothing new: while W is pending it joins the same
  waiter; after completion it returns the same success or
  `ERR_WRITE_CONFIRM_TIMEOUT` outcome.
- Dedup entries are scoped by the authenticated Node identity so IDs cannot
  collide across nodes.
- A reused `MutationID` must carry the same operation, key/value/TTL, write mode,
  W and confirmation policy. A mismatched retry is rejected as a terminal
  invalid request and never changes the original mutation or waiter.

(The profile/acknowledgement rules those requests obey are frozen in **Profile
rules** above.)

## Cross-component verification

- [ ] Fake and real Hub pass the same Node client contract suite.
- [ ] RPC and WAL golden vectors detect incompatible field, version and checksum
  changes.
- [ ] Timeout/retry cannot duplicate a mutation.
- [ ] A retry during or after W waiting observes the original waiter and final
  W outcome; mismatched reuse of a `MutationID` fails terminally.
- [ ] Sync fences earlier Fast writes and recovery has no sequence holes.
- [ ] W and Fast/Sync combinations have independent result semantics.
- [ ] Memory restart loses state safely through generation change.
- [ ] Durable restart preserves Sync-acknowledged state; loss of a Fast tail
  forces a new generation and Node revalidation.

**Exit:** transport and storage-profile changes preserve cache consistency while
making restart durability explicit.

# Common P2 — Control protocol (gRPC)

**Depends on:** [COMMON_PHASE_01_TEST_CONTRACT.md](COMMON_PHASE_01_TEST_CONTRACT.md).

## Transport and schema

Control is the **gRPC** service `gossipcache.v1.Control` on the same mTLS
listener as Data (port **7400**):

```text
rpc Connect(stream ControlClientMessage) returns (stream ControlServerMessage);
```

| Artifact | Location |
|----------|----------|
| IDL | [`api/proto/gossipcache/v1/control.proto`](../../../api/proto/gossipcache/v1/control.proto) |
| Generated stubs | `api/gen/gossipcache/v1` |
| Domain validation + conversion | `internal/control` |

Regenerate: `make proto`.

```text
stream_id       = partition_id under hub_generation
version         = (partition_id, commit_sequence)
stream_sequence = contiguous delivery order for the partition stream
```

Values never ride this plane (SEMANTICS). Transport receipt is gRPC/HTTP2 flow
control; only **application apply** advances watermarks and W.

### Message size (frozen)

| Constant | Value | Meaning |
|----------|------:|---------|
| `MaxControlMessageBytes` | 3 MiB | Max encoded `ControlClientMessage` / `ControlServerMessage` |
| gRPC default `MaxRecvMsgSize` / `MaxSendMsgSize` | 4 MiB | Must remain ≥ `MaxControlMessageBytes` |
| `MaxBatchEvents` | 4096 | Secondary cap; encoded size is binding |

A domain-valid `InvalidationBatch` must satisfy
`EstimatedEncodedSize() ≤ MaxControlMessageBytes`. Hubs **split** publish so
each stream message stays under the limit. Nodes reject oversized batches fail
closed. This prevents a legal batch (~16 MiB of keys at max event count) from
terminating the stream with gRPC `ResourceExhausted`.

## Message catalog

### Client → server (`ControlClientMessage`)

| Message | Role |
|---------|------|
| `Hello` | Node id, protocol range, `cluster_id`, expected generation, resume watermarks |
| `Subscribe` | Partition interest under hub generation |
| `StreamAcknowledgement` | Application applied through sequence |
| `ReplayRequest` | Inclusive gap fill |
| `InvalidateConfirm` | Peer apply confirm for hub-aggregated W |

### Server → client (`ControlServerMessage`)

| Message | Role |
|---------|------|
| `InvalidationBatch` | Contiguous hub-numbered invalidation events (size-limited) |
| `StreamCheckpoint` | Idle stream head + hub generation |
| `ReplayUnavailable` | Retained window when replay cannot fill a gap |
| `ControlError` | Application stream error (`STATUS_ERR_RATE_LIMITED`, `STATUS_ERR_BAD_GENERATION`, `STATUS_ERR_INVALID_ARGUMENT`, `STATUS_ERR_INTERNAL`) |

Connect-time hard rejects (TLS, accept budget) use gRPC status codes
(`PermissionDenied`, `ResourceExhausted`) without an application body.

## Delivery rules

- [x] Hub assigns version and stream sequences; Node never renumbers them.
- [x] Application ack means applied state (not mere gRPC receipt).
- [x] Encoded message size bounded to `MaxControlMessageBytes`.
- [ ] Reconnect exchanges subscribed partition watermarks.
- [ ] Gap requests replay; expired replay requires held-key reconciliation.
- [x] Idle checkpoints carry stream head and `hub_generation`.
- [ ] Backpressure is bounded per subscriber (gRPC stream + hub send queue).
- [ ] W confirmations deduplicate Node identity and exclude the writer.

## Implementation detail

### Invalidation event (domain shape)

```go
type InvalidationEvent struct {
    StreamSequence uint64
    Key            []byte
    Version        wire.VersionTag  // (partition_id, commit_sequence)
    Kind           RecordKind       // Value | Tombstone
    MutationID     wire.MutationID  // W correlation and self-invalidation dedup
}
```

Batches must be non-empty and contiguous by `StreamSequence`. Every event
version partition must equal the batch `stream_id`. Encoded size must stay
within `MaxControlMessageBytes`. `ReplayUnavailable` reports the retained
window so the consumer can distinguish an expired range without accepting a gap.

### Delivery algorithm

- **Numbering:** hub assigns `Version.Sequence` (commit order) and
  `StreamSequence` (delivery order) once; nodes echo, never renumber.
- **Application ack:** `StreamAcknowledgement` advances the subscriber
  application watermark and releases connection-local delivery buffers. It does
  not shorten the shared replay window.
- **Reconnect:** `Hello` carries `cluster_id`, `expected_hub_generation`, and
  `(subscribed_partition, applied_watermark, hub_generation)` per partition;
  the hub resumes from `applied_watermark + 1` or answers `ReplayUnavailable`
  if below the retained window.
- **Gaps:** a node detecting `StreamSequence > expected` requests replay of the
  hole; expired replay => `RECONCILIATION_REQUIRED` (P4 held-key path).
- **Checkpoints:** every `CheckpointInterval` (default 1s) of stream idleness.
- **Liveness:** connection state + checkpoints (no separate subscription lease).
- **Backpressure:** per-subscriber bounded queue (`SubscriberQueue`, default
  4096 events); overflow drops the *subscriber*, not the partition commit.
- **Retention:** disconnect and application acks do not shorten
  `ReplayRetention`.
- **W confirmation:** `InvalidateConfirm` dedups by `NodeID`; writer never
  counts.

## Cross-component verification

- [ ] Hub and Node use the same generated stubs and `internal/control` helpers.
- [ ] One Hub / two Nodes: ordering, replay, stall detection and W timeout.
- [ ] Stopped checkpoints cause freshness failure and reconnect.
- [x] Domain validation, size limit, and protobuf round-trips in `internal/control`.
- [ ] Oversized batch is rejected without gRPC ResourceExhausted from the peer.

Pure subscriber-to-subscriber relay/fanout is **not** v1.

**Exit:** delivery loss cannot remain silent while a Node reports ready.

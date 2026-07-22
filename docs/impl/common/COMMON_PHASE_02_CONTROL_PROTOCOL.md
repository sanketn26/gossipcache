# Common P2 — Control protocol

**Depends on:** [COMMON_PHASE_01_TEST_CONTRACT.md](COMMON_PHASE_01_TEST_CONTRACT.md).

## Frame and messages

Use a versioned length-prefixed frame with fixed magic, message type, bounded
payload length and fail-closed schema handling. Freeze golden vectors for hello,
subscribe, `InvalidationBatch`, `HopFrameAck`, `StreamAcknowledgement`,
`StreamCheckpoint`, `ReplayRequest`, replay-unavailable and
`InvalidateConfirm`.

```text
stream_id       = partition_id under hub_generation
version         = (partition_id, commit_sequence)
stream_sequence = contiguous delivery order for the partition stream
```

## Delivery rules

- [ ] Hub assigns version and stream sequences; Node never renumbers them.
- [ ] Hop ack means decoded receipt only; application ack means applied state.
- [ ] Reconnect exchanges subscribed partition watermarks.
- [ ] Gap requests replay; expired replay requires held-key reconciliation.
- [ ] Idle checkpoints carry stream head and `hub_generation`.
- [ ] Backpressure is bounded per subscriber.
- [ ] W confirmations deduplicate Node identity and exclude the writer.

## Implementation detail

### Frame layout (`internal/control`)

Fixed 16-byte header, big-endian, followed by a bounded payload:

```text
offset  size  field
0       4     magic = 0x47435331 ("GCS1")
4       2     protocol_version
6       2     msg_type
8       4     payload_len (<= MaxControlPayload = 1 MiB; over => close)
12      4     header_crc32c (over bytes 0..11)
16      N     payload (msg-specific, length-delimited fields)
```

`msg_type` enum: `Hello=1`, `Subscribe=2`, `InvalidationBatch=3`,
`HopFrameAck=4`, `StreamAcknowledgement=5`, `StreamCheckpoint=6`,
`ReplayRequest=7`, `ReplayUnavailable=8`, `InvalidateConfirm=9`. Unknown type or
bad magic/crc => close the connection (fail-closed). Golden encodings for each
live in `internal/control/testdata/`.

### Message payloads

```go
type InvalidationBatch struct {
    StreamID       uint32           // = partition_id under hub_generation
    HubGeneration  uint64
    Events         []InvalidationEvent // contiguous by StreamSequence
}
type InvalidationEvent struct {
    StreamSequence uint64           // contiguous per stream; hub-assigned
    Key            []byte
    Version        wire.VersionTag  // (partition_id, commit_sequence)
    Kind           RecordKind       // Value | Tombstone
    MutationID     wire.MutationID  // for W correlation and self-invalidation dedup
}
type StreamCheckpoint struct {
    StreamID uint32; HubGeneration uint64; StreamHead uint64 // idle liveness
}
type StreamAcknowledgement struct { StreamID uint32; AppliedThrough uint64 }
type ReplayRequest struct { StreamID uint32; FromSequence, ToSequence uint64 }
type InvalidateConfirm struct { StreamID uint32; StreamSequence uint64; NodeID uint64 }
```

### Delivery algorithm

- **Numbering:** hub assigns `Version.Sequence` (commit order) and
  `StreamSequence` (delivery order) once; nodes echo, never renumber.
- **Two ack levels:** `HopFrameAck` = frame decoded (transport receipt);
  `StreamAcknowledgement` = state-machine applied (advances the subscriber
  application watermark and releases connection-local delivery buffers). They
  are distinct and never conflated; neither shortens the shared replay window.
- **Reconnect:** `Hello` carries `(subscribed_partition, applied_watermark,
  hub_generation)` per partition; the hub resumes from `applied_watermark + 1`
  or answers `ReplayUnavailable` if below the retained window.
- **Gaps:** a node detecting `StreamSequence > expected` requests replay of the
  hole; expired replay => the node marks `RECONCILIATION_REQUIRED` (P4 held-key
  path) rather than accepting a gap.
- **Checkpoints:** emitted every `CheckpointInterval` (default 1s) of stream
  idleness so a silent stall is visible as stale `StreamHead`.
- **Liveness model:** v1 uses connection state plus checkpoints, not a separate
  subscription lease. Missing checkpoints past the configured freshness timeout
  gates readiness and forces reconnect/replay.
- **Backpressure:** each subscriber has a bounded send queue
  (`SubscriberQueue`, default 4096 events); overflow drops the *subscriber*, not
  the partition commit — the node reconnects and replays.
- **Retention guarantee:** disconnect and application acknowledgements do not
  shorten `ReplayRetention`; they release connection-local buffers only. Events
  leave the shared replay ring solely at the configured time/count boundary.
- **W confirmation:** `InvalidateConfirm` dedups by `NodeID`; the writer node's
  own confirm never counts.

## Cross-component verification

- [ ] Hub and Node consume identical golden vectors.
- [ ] One Hub/two Nodes tests ordering, replay, stall detection and W timeout.
- [ ] Stopped checkpoints cause freshness failure and reconnect without relying
  on an independent subscription lease timer.

Pure subscriber-to-subscriber relay/fanout is not implemented in v1. Every
watermark in this contract belongs to a direct Hub subscriber; a later relay
role requires a new common protocol contract.

**Exit:** delivery loss cannot remain silent while a Node reports ready.

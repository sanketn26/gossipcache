# Hub P2 — Control-stream origin

**Depends on:** [HUB_PHASE_01_NODE_SM_SUPPORT.md](HUB_PHASE_01_NODE_SM_SUPPORT.md)
and its injected commit-event seam.

**Common contract:** [COMMON_PHASE_02_CONTROL_PROTOCOL.md](../common/COMMON_PHASE_02_CONTROL_PROTOCOL.md).

## Functional work

- [ ] Maintain one ordered stream per internal partition.
- [ ] Publish invalidations only after commit eligibility.
- [ ] Track direct subscribers and per-subscriber backpressure (no
  subscription leases; liveness is connection + checkpoints).
- [ ] Retain bounded replay ranges and answer replay requests.
- [ ] Emit idle `StreamCheckpoint` messages.
- [ ] Retain shared replay strictly to the configured time/count boundary;
  application watermarks release connection-local buffers only.
- [ ] Aggregate `W` confirmations by distinct node ID; exclude writer.
- [ ] Preserve committed mutations when `W` times out.

## Implementation detail

### Stream origin (`internal/control`, hub side)

One ordered stream per internal partition. The partition commit path and the
stream publish share the partition lock so a mutation's `stream_sequence` is
assigned in the same critical section as its `VersionTag` — no reordering
window.

```go
type partitionStream struct {
    mu          sync.Mutex
    streamSeq   uint64                  // last assigned stream_sequence
    ring        *replayRing             // bounded retained events for replay
    subs        map[uint64]*subscriber  // node_id -> subscriber
    wConfirms   map[uint64]*wWaiter      // stream_sequence -> pending W aggregation
}

type subscriber struct {
    nodeID    uint64
    sendQ     chan InvalidationBatch // bounded (SubscriberQueue, default 4096)
    appliedTo uint64                 // last StreamAcknowledgement watermark
}
```

### Publish and backpressure

- After commit eligibility, the event is appended to `ring` and fanned out to
  each subscriber's `sendQ`. A full `sendQ` **drops that subscriber** (close +
  mark for reconnect) — it never blocks the partition commit.
- Idle `StreamCheckpoint{StreamHead, HubGeneration}` is emitted every
  `CheckpointInterval` (default 1s) so a silent stall is observable.

### Replay retention

- `replayRing` retains up to `ReplayRetention` (default 10 min or 1M events,
  whichever first). `ReplayRequest[from,to]` inside the window is served in
  order; below the window returns `ReplayUnavailable`, pushing the node to
  held-key reconciliation (P4).
- `ReplayRetention` is an availability guarantee independent of the currently
  connected subscriber set. Dropping/disconnecting a subscriber cannot make an
  event eligible for early reclamation. The ring keeps each event until the
  configured time or count boundary evicts it; a reconnect inside the retained
  range can therefore replay from its presented watermark.
- Application watermarks release per-connection delivery buffers and provide
  lag metrics, but v1 does not use them to shorten the shared replay window.
- v1 has no subscriber relay role and no separate subscription lease. Idle
  checkpoints provide freshness; connection loss triggers replay on reconnect.

### W aggregation

```go
type wWaiter struct {
    need     int
    seen     map[uint64]struct{} // confirming node_ids, writer excluded
    done     chan struct{}
    deadline time.Time
}
```

The Set RPC registers the waiter under the event's `stream_sequence` before the
event is published (closing the confirm-before-waiter race). `InvalidateConfirm`
frames add `node_id` to `seen`; reaching `need` completes the RPC. On deadline,
the RPC returns `ErrWriteConfirmTimeout` — the commit is untouched.

The waiter and its final outcome are retained by the mutation dedup entry for
the dedup window. An identical retry joins the pending waiter or replays its
final result. Reuse of the same `MutationID` with different mutation or W
options is rejected terminally; it cannot create or replace a waiter.

## Verification

- [ ] Ordered publish and subscriber-isolation unit tests.
- [ ] Gap/replay, expired replay and silent-idle checkpoint tests.
- [ ] Dropping the only lagging subscriber does not reclaim events still inside
  the configured replay window; reconnect replays them from its watermark.
- [ ] Confirm-before-waiter, duplicate confirm and writer-exclusion tests.
- [ ] Slow subscriber cannot block a partition commit.

**Exit:** two node consumers converge, replay a forced gap and satisfy or time
out `W` without losing the committed mutation.

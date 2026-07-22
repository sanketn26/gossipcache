# Node P2 — Hub stream consumer

**Depends on:** [NODE_PHASE_01_STATE_MACHINE.md](NODE_PHASE_01_STATE_MACHINE.md).

**Common contract:** [COMMON_PHASE_02_CONTROL_PROTOCOL.md](../common/COMMON_PHASE_02_CONTROL_PROTOCOL.md).

## Functional work

- [ ] Subscribe on first partition interest; never auto-unsubscribe in v1.
- [ ] Apply ordered batches and maintain contiguous application watermarks.
- [ ] Acknowledge only after state-machine application.
- [ ] Detect gaps and request replay.
- [ ] Mark reconciliation required when replay is unavailable.
- [ ] Track checkpoint freshness for required subscriptions.
- [ ] Send `InvalidateConfirm` after apply for W-tracked events.
- [ ] Treat a same-version self-invalidation idempotently.

## Implementation detail

### Consumer structure (`internal/control`, node side)

One owned goroutine per subscribed partition stream plus a shared decode loop.

```go
type streamConsumer struct {
    partition   uint32
    appliedTo   uint64        // contiguous applied watermark (drives StreamAcknowledgement)
    expected    uint64        // next stream_sequence to apply
    lastCheck   time.Time     // last StreamCheckpoint receipt
    gen         uint64        // hub_generation for this subscription
}
```

### Apply loop

```text
on InvalidationBatch:
  for ev in batch (ordered):
    if ev.StreamSequence == expected:
        stateMachine.apply(ev)          // may raise ceiling / mark STALE
        appliedTo = ev.StreamSequence; expected++
        if ev tracked for W: send InvalidateConfirm(node_id)
    elif ev.StreamSequence < expected:  // duplicate/self, idempotent
        continue
    else:                                // gap
        send ReplayRequest[expected, ev.StreamSequence-1]; break
  send StreamAcknowledgement(appliedTo)
```

- **Ack ordering:** `HopFrameAck` on decode is separate from the
  `StreamAcknowledgement` sent only after state-machine apply — the hub frees
  replay on the latter.
- **Subscription lifecycle:** subscribe on first interest in a partition; v1
  never auto-unsubscribes, so interest only grows.
- **Reconnect:** `Hello` replays `(partition, appliedTo, gen)`; the hub resumes
  from `appliedTo+1` or answers `ReplayUnavailable`.
- **Replay exhausted:** on `ReplayUnavailable` the consumer sets
  `RECONCILIATION_REQUIRED` for the partition (P4 held-key path) rather than
  skipping the gap.
- **Freshness:** if no batch or `StreamCheckpoint` arrives within
  `StreamFreshnessTimeout` (default 3s), the partition is `STREAM_FRESHNESS_UNKNOWN`
  and readiness drops.
- **Self-invalidation:** an event whose version equals the writer's already
  installed record is applied idempotently (no STALE churn).

## Verification

- [ ] Reconnect from exchanged watermarks.
- [ ] Gap, replay overflow and silent-stall tests.
- [ ] Unknown-key invalidations do not grow the slot map.
- [ ] An invalidation racing a fetch prevents stale installation.

**Exit:** the node reliably consumes hub truth and cannot remain ready while a
required stream is stale or unreconciled.

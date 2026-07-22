# Node P4 — Reconciliation, health and deployment

**Depends on:** [NODE_PHASE_03_RPC_CLIENT.md](NODE_PHASE_03_RPC_CLIENT.md).

**Common contract:** [COMMON_PHASE_04_OPERATIONS.md](../common/COMMON_PHASE_04_OPERATIONS.md).

## Functional work

- [ ] Reconcile only held keys against hub summaries.
- [ ] Keep readiness false for gaps, stale checkpoints, missing routes and
  generation revalidation.
- [ ] Separate liveness, startup and consistency readiness.
- [ ] Make start/stop idempotent and terminate every goroutine.
- [ ] Provide probe wiring for application Deployments.
- [ ] Install Common P4 no-op metric hooks for readiness, gaps, checkpoint age,
  replay and reconciliation.

## Implementation detail

### Readiness composition

The node's `/readyz` reports the worst `ReadyReason` across subscribed
partitions:

```go
func (n *Node) readyReason() ReadyReason {
    reason := Ready
    for _, p := range n.partitions {
        reason = higherPriority(reason, p.readyReason())
    }
    return reason
}
```

Readiness never returns `Ready` while any required stream is gapped, stale, or a
generation change is being revalidated. `higherPriority` implements the frozen
Common P4 ordering, so map/partition iteration order cannot change the response.

### Held-key reconciliation

- Walk locally held keys per partition in hash order, sending `HeldKeyDigest`
  batches (`DigestBatch`, default 1024) to the hub summary API.
- Drop/revalidate any local entry the hub reports older, different-generation, or
  unknown. Absent keys are **never** fetched — reconciliation restores
  correctness, not warmth.
- Triggered by `ReplayUnavailable`, generation change, or operator request.

### Lifecycle

- `Start`/`Stop` idempotent; `Stop` cancels the root context and joins every
  stream/refresh/reconcile goroutine (verified under `-race`).
- Probe wiring exposes `/livez`, `/startupz`, `/readyz` on the node management
  listener for application Deployments.

## Verification

- [ ] Replay overflow followed by held-key reconciliation restores readiness.
- [ ] Hub loss, stream stall and generation change report distinct reasons.
- [ ] Multiple simultaneous partition failures always produce the same primary
  reason regardless of partition insertion/iteration order.
- [ ] Shutdown and repeated Stop tests under the race detector.
- [ ] Minimum signals follow each readiness transition and exporter failure does
  not delay Get/apply/reconciliation.

**Exit:** readiness accurately states whether the embedded cache can safely
serve, without warming absent keys or leaking goroutines.

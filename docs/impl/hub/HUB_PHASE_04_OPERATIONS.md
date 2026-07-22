# Hub P4 — Reconciliation, health and deployment

**Depends on:** [HUB_PHASE_03_STORAGE.md](HUB_PHASE_03_STORAGE.md).

**Common contract:** [COMMON_PHASE_04_OPERATIONS.md](../common/COMMON_PHASE_04_OPERATIONS.md).

## Functional work

- [ ] Expose per-partition held-key summary/range-digest APIs.
- [ ] Separate `/livez`, `/startupz` and `/readyz` semantics.
- [ ] Gate readiness on empty-generation initialization in memory mode, or
  completed recovery/storage writability in durable mode, plus route ownership
  and compatible RPC/control listeners.
- [ ] Surface bounded readiness reason codes.
- [ ] Provide StatefulSet, PVC and service definitions for hub ports.
- [ ] Define replay-retention and reconciliation operational limits.
- [ ] Install Common P4 no-op metric hooks for readiness, replay,
  reconciliation and durable-backend errors.

## Implementation detail

### Readiness gate

`/readyz` computes a single `ReadyReason` (common P4 enum) from:

```go
func (h *Hub) readyReason() ReadyReason {
    switch {
    case h.profile == StorageDurable && !h.recovered.Load(): return RecoveryInProgress
    case h.profile == StorageDurable && !h.store.Healthy():   return DurabilityDegraded
    case !h.routesOwned():                                    return RouteUnavailable
    case !h.listenersHealthy():                               return RouteUnavailable
    default:                                                  return Ready
    }
}
```

Memory mode is ready once partitions are initialized under the fresh
`hub_generation`; durable mode is ready only after `Recover()` completes and the
store is writable.

### Held-key summary API

Serves node reconciliation without warming: given a `HeldKeyDigest` batch, reply
with the authoritative version per key hash (or "absent"). Bounded to
`DigestBatch` (default 1024) keys per request; the hub never pushes keys the node
does not already hold.

### Deployment artifacts (`deployments/k8s`)

- Memory default: `Deployment` (or `StatefulSet` without a required PVC), pod
  anti-affinity, `readinessProbe: /readyz`, `livenessProbe: /livez`,
  `startupProbe: /startupz`.
- Durable opt-in: `StatefulSet` + `volumeClaimTemplates` PVC mounted at
  `DataDir`; readiness stays 503 (`RecoveryInProgress`) until WAL replay
  completes, keeping traffic off an unrecovered authority.
- Operational limits surfaced as config/flags: `ReplayRetention`,
  `MaxReconcileKeys`, WAL segment/compaction thresholds.

## Verification

- [ ] Reconciliation after replay expiry.
- [ ] Probes distinguish dependency/recovery failure from deadlock.
- [ ] Kubernetes manifest validation and restart tests for both profiles.
- [ ] Minimum signals follow each readiness transition and remain non-blocking
  when the exporter is absent or failing.

**Exit:** orchestration never routes to an unrecovered authority and nodes can
reconcile held keys after replay loss.

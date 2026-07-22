# Common P4 — Reconciliation and health contract

**Depends on:** [COMMON_PHASE_03_DATA_PROTOCOL.md](COMMON_PHASE_03_DATA_PROTOCOL.md).

## Shared health meanings

- `/livez`: owned event loops advance; dependency health is excluded.
- `/startupz`: local recovery/initialization completed.
- `/readyz`: the component can safely perform its consistency role.

## Decisions

- [ ] Freeze bounded held-key digest/summary messages.
- [ ] Freeze readiness reason codes and generation-change behavior.
- [ ] Define replay retention and reconciliation limits.
- [ ] Define Hub and Node Kubernetes probe expectations.

## Implementation detail

### Readiness reason codes

Frozen closed enum, surfaced on `/readyz` body and metrics (both components):

```go
type ReadyReason uint16
const (
    Ready                 ReadyReason = iota
    RecoveryInProgress                // durable hub replaying WAL
    GenerationRevalidating            // hub_generation changed; discarding old entries
    StreamGap                         // missing stream range, replay pending
    StreamFreshnessUnknown            // no checkpoint within freshness timeout
    ReconciliationRequired            // replay expired; held-key digest pending
    RouteUnavailable                  // partition owner/listener not ready
    DurabilityDegraded                // durable backend unhealthy (hub)
)
```

`/readyz` returns 200 only for `Ready`; any other reason is 503 with the code in
the body. `/livez` ignores all of these (it only checks owned loops advance).

The HTTP/metric string encoding is frozen as SCREAMING_SNAKE_CASE:
`READY`, `RECOVERY_IN_PROGRESS`, `GENERATION_REVALIDATING`, `STREAM_GAP`,
`STREAM_FRESHNESS_UNKNOWN`, `RECONCILIATION_REQUIRED`, `ROUTE_UNAVAILABLE` and
`DURABILITY_DEGRADED`. Go identifiers are never emitted directly.

When multiple reasons apply, components report one deterministic primary reason
using this highest-first priority: `RECOVERY_IN_PROGRESS`,
`GENERATION_REVALIDATING`, `RECONCILIATION_REQUIRED`, `STREAM_GAP`,
`STREAM_FRESHNESS_UNKNOWN`, `ROUTE_UNAVAILABLE`, `DURABILITY_DEGRADED`, then
`READY`. Metrics may additionally expose every active bounded reason, but the
HTTP body and primary-reason gauge use this ordering.

### Held-key reconciliation (anti-entropy)

Reconciliation compares only keys the requester currently holds — it never warms
absent keys.

```go
type HeldKeyDigest struct {
    PartitionID uint32
    HubGeneration uint64
    Entries     []KeyVersion // {KeyHash uint64, Version wire.VersionTag}
}
```

- The node sends per-partition digests bounded to `DigestBatch` (default 1024)
  keys, walking held keys in hash order across batches.
- The hub replies with the authoritative version per key hash; the node drops or
  revalidates any local entry that is older, a different generation, or unknown
  to the hub.
- Trigger conditions: replay window expired, `hub_generation` change, or
  operator-forced reconcile. Limits: `ReplayRetention` (default 10 min or 1M
  events per stream, whichever first) and `MaxReconcileKeys` guard rails.

### Probe expectations

- `/livez`, `/startupz`, `/readyz` on the management port (`8081` node,
  `8081` hub mgmt), plain HTTP, no auth, bound to localhost/pod-local.
- Kubernetes: `startupProbe` -> `/startupz`, `livenessProbe` -> `/livez`,
  `readinessProbe` -> `/readyz`; readiness gating keeps traffic off an
  unrecovered/ungapped component.

### P4 minimum safety instruments

P4 installs no-op-first hooks and the minimum signals needed to trust probes;
P5 owns full exporters, dashboards, paging rules and catalog validation.

| Signal | Required labels |
|--------|-----------------|
| `gossipcache_ready_reason` | bounded `component`, `reason` |
| `gossipcache_stream_gap_total` | bounded `partition` |
| `gossipcache_replay_total` | `result={served,unavailable}` |
| `gossipcache_checkpoint_age_seconds` | bounded `partition` |
| `gossipcache_reconcile_total` | bounded `trigger`, `result` |
| `gossipcache_durability_error_total` | bounded `operation` (durable Hub only) |

Instrumentation calls must be non-blocking and safe with a no-op provider.
Exporter failure cannot change readiness, block L1 access or block Hub commit.

## Cross-component verification

- [ ] Replay overflow reconciles only held keys.
- [ ] Hub restart, stream stall, route loss and generation change expose the
  correct readiness transitions.
- [ ] Minimum-signal tests map every readiness fault to its frozen wire string
  and prove a failed exporter cannot block consistency paths.

**Exit:** readiness returns only after authoritative comparison and fresh stream
evidence, without warming absent keys.

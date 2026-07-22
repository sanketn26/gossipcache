# Node P5 — Observability

**Depends on:** [NODE_PHASE_04_OPERATIONS.md](NODE_PHASE_04_OPERATIONS.md).

**Common contract:** [COMMON_PHASE_05_OBSERVABILITY.md](../common/COMMON_PHASE_05_OBSERVABILITY.md).

## Functional work

- [ ] Measure local hit/miss/stale outcomes and hub fetch/write latency.
- [ ] Count state transitions, rejected stale fetches and invalidation applies.
- [ ] Expose bounded watermark lag, checkpoint age and reconciliation state.
- [ ] Trace a miss/write across node RPC and local installation.
- [ ] Avoid raw keys, values, credentials and unbounded labels.
- [ ] Contribute Node-cache and stream/reconciliation dashboards, recording
  rules, alerts and runbook links to the Common P5 deliverables.

## Implementation detail

### Node-owned instruments

Implements the common P5 vocabulary from the embedded side; local-hit
instrumentation must stay within the < 3% overhead budget.

- `gossipcache_local_result_total{result=hit|miss|stale}` — counter on the read
  path; the hit counter is the proof hot reads never touch the hub.
- `gossipcache_rpc_duration_seconds{op,status,component="node"}` for hub
  fetch/write latency.
- `gossipcache_state_transition_total{from,to}`,
  `gossipcache_stale_fetch_rejected_total`,
  `gossipcache_invalidation_applied_total`.
- Per-partition gauges: `..._stream_watermark_lag`, `..._checkpoint_age_seconds`,
  `..._reconcile_state`.
- The `ReadyReason` is exported both on `/readyz` and as a
  `gossipcache_ready_reason` gauge for alerting.

### Traces

`node.set`/`node.get` root spans mint the `trace_id` propagated to the hub;
`node.apply` closes the loop on invalidation. Raw keys, values, credentials and
unbounded labels are excluded everywhere.

## Verification

- [ ] Deterministic metric and readiness-reason tests.
- [ ] Dashboards distinguish local pressure from hub/stream failure.
- [ ] Cardinality allowlist covers every Node instrument and label value.
- [ ] Slow/failed telemetry export cannot block local hits, state transitions,
  invalidation apply or shutdown; bounded queue overflow is observable.

**Exit:** operators can tell why a node is stale or unready while local-hit
instrumentation remains safe and low overhead.

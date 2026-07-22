# Common P5 — Observability contract

**Depends on:** [COMMON_PHASE_04_OPERATIONS.md](COMMON_PHASE_04_OPERATIONS.md).

- [ ] Freeze metric names, units and bounded attributes for RPC, mutation,
  stream, freshness, replay and reconciliation.
- [ ] Define trace correlation from Node API through Hub commit and invalidation.
- [ ] Define structured event names for gaps, corruption, generation changes and
  rejected stale fetches.
- [ ] Ban values, credentials, TLS material and raw high-cardinality keys.
- [ ] Set instrumentation overhead budgets.
- [ ] Require bounded `storage_profile` separation and durable-only sync,
  queueing, compaction, recovery and disk-capacity signals.
- [ ] Require bounded `write_mode` separation plus Fast backlog and Sync fence
  latency; never label by request ID or key.
- [ ] Ship dashboards, recording rules, SLO indicators, alerts and linked
  runbooks under `deployments/observability/`.
- [ ] Enforce the metric/label allowlist in CI and validate every production
  fault against a signal and readiness outcome.
- [ ] Keep telemetry queues memory-bounded; export must never block L1 hits,
  Hub commits, stream apply or shutdown.

## Implementation detail

### Metric namespace and cardinality

Prometheus-style names under the `gossipcache_` prefix. Bounded label sets only;
never label by key, value, `MutationID` or raw address.

| Metric | Type | Bounded labels |
|--------|------|----------------|
| `gossipcache_rpc_duration_seconds` | histogram | `op={get,set,delete}`, `status`, `component` |
| `gossipcache_mutation_total` | counter | `op`, `write_mode={fast,sync}`, `status` |
| `gossipcache_stream_events_total` | counter | `direction={publish,apply}`, `partition` (bounded) |
| `gossipcache_stream_watermark_lag` | gauge | `partition` |
| `gossipcache_checkpoint_age_seconds` | gauge | `partition` |
| `gossipcache_subscriber_queue_depth` | gauge | bounded `partition` |
| `gossipcache_subscriber_queue_capacity` | gauge | bounded `partition` |
| `gossipcache_stream_gap_total` | counter | bounded `partition` |
| `gossipcache_replay_total` | counter | `result={served,unavailable}` |
| `gossipcache_reconcile_total` | counter | `trigger` |
| `gossipcache_singleflight_waiters` | gauge | `component="node"` |
| `gossipcache_l1_resident_bytes` | gauge | none |
| `gossipcache_l1_evictions_total` | counter | bounded `reason` |
| `gossipcache_hub_generation_high` / `_low` | gauges | `component` (two uint32 halves as values; generation is never a label) |
| `gossipcache_version_regression_total` | counter | `component` |
| `gossipcache_reconcile_keys_total` | counter | `result={compared,dropped,revalidated}` |
| `gossipcache_partition_version_head` | gauge | bounded `partition` |
| `gossipcache_stream_head` | gauge | bounded `partition` |
| `gossipcache_w_waiters` | gauge | bounded `partition` |
| `gossipcache_w_timeout_total` | counter | none |
| `gossipcache_hub_persist_queue_depth` | gauge | `partition` (durable only) |
| `gossipcache_hub_persist_queue_capacity` | gauge | `partition` (durable only) |
| `gossipcache_hub_persist_queue_oldest_seconds` | gauge | `partition` (durable only) |
| `gossipcache_hub_sync_fence_seconds` | histogram | `partition` (durable only) |
| `gossipcache_hub_recovery_seconds` | histogram | none (durable only) |
| `gossipcache_hub_compaction_seconds` | histogram | none (durable only) |
| `gossipcache_hub_disk_bytes` | gauge | none (durable only) |

`partition` is bounded because `PartitionCount` is fixed; if it exceeds
`MaxLabelPartitions` (default 64) partitions collapse to an `"agg"` label.

### Trace correlation

- One `trace_id` is minted at the Node public API call and propagated in the RPC
  frame header and the resulting `InvalidationEvent`, so a write can be followed
  from `Client.Set` → hub commit → stream publish → peer apply.
- Span names: `node.set`, `rpc.mutation`, `hub.commit`, `hub.publish`,
  `node.apply`. Durable hubs add `hub.persist` / `hub.sync_fence`.

### Structured events (logs)

Frozen event names, values excluded: `stream_gap`, `replay_unavailable`,
`generation_change`, `stale_fetch_rejected`, `store_corruption`,
`durability_degraded`. Each carries `partition`, `hub_generation`, and bounded
sequence numbers only.

### Overhead budget

Instrumentation must add < 3% to the local-hit path (measured against the P8
hit benchmark); high-cardinality/debug detail lives behind an off-by-default
build tag or sampling.

Export uses bounded asynchronous queues with drop/coalesce counters. A missing,
slow or failed collector never runs synchronously on the local-hit, partition
commit or stream-apply path and never changes operation success.

### P4/P5 ownership and deliverables

- P4 owns only probe semantics, readiness strings, no-op hooks and minimum
  safety instruments.
- P5 owns Prometheus/OTel export, the complete reviewed catalog, four dashboards
  (Node cache, Hub/RPC, stream/reconciliation, durability/recovery), recording
  rules, SLO burn-rate alerts, runbook links and syntax tests.
- `internal/obs` provides consumer-facing `Meter`, `Tracer` and structured-event
  interfaces with no-op defaults; components do not depend on an exporter SDK.

### Fault-to-signal exit matrix

| Fault | Required evidence |
|-------|-------------------|
| Dropped stream range | gap counter/event, replay trace, non-ready reason |
| Stopped checkpoints | checkpoint-age breach, `STREAM_FRESHNESS_UNKNOWN` alert |
| Replay expired | reconciliation counter/event and readiness transition |
| Generation change | generation event/info and revalidation readiness |
| Durable queue/fence failure | queue/fence metrics, durability event and alert |
| Hub route/listener loss | RPC errors and `ROUTE_UNAVAILABLE` |
| Telemetry backend failure | bounded drop counter; cache/commit/apply still progress |

CI enumerates every instrument and allowed label/value set, rejects raw keys,
addresses and IDs, validates dashboard/alert syntax, and runs this matrix with
deterministic fault hooks.

**Exit:** one bounded trace/metric vocabulary explains the complete write and
invalidation path; dashboards, alerts and runbooks pass syntax/fault tests; and
telemetry failure cannot impede cache correctness or availability.

# Hub P5 — Observability

**Depends on:** [HUB_PHASE_04_OPERATIONS.md](HUB_PHASE_04_OPERATIONS.md).

**Common contract:** [COMMON_PHASE_05_OBSERVABILITY.md](../common/COMMON_PHASE_05_OBSERVABILITY.md).

## Functional work

- [ ] Measure RPC/commit latency in both profiles and sync latency/failures in
  durable mode.
- [ ] Expose bounded partition version and stream heads.
- [ ] Instrument subscribers, replay, checkpoints, backpressure and W waits.
- [ ] Emit structured recovery, corruption and generation events without values.
- [ ] Correlate mutation RPC, profile commit and invalidation publication traces.
- [ ] Label/profile dashboards by bounded `storage_profile` and show durable
  queueing, group-commit, compaction, recovery and disk-capacity pressure.
- [ ] Separate Fast and Sync counts/latency; expose Fast persistence queue depth,
  oldest queued age, Sync fence wait and durability-unavailable failures.
- [ ] Contribute Hub/RPC and durability/recovery dashboards, recording rules,
  alerts and runbook links to the Common P5 deliverables.

## Implementation detail

### Hub-owned instruments

Implements the common P5 vocabulary from the authority side:

- `gossipcache_rpc_duration_seconds{op,status,component="hub"}` and
  `gossipcache_mutation_total{op,write_mode,status}` around the partition
  commit.
- Per-partition gauges: `..._partition_version_head`, `..._stream_head`,
  `..._subscribers`, `..._subscriber_queue_depth`, `..._w_waiters`.
- Durable-only: `..._hub_persist_queue_depth`, `..._hub_persist_queue_oldest_seconds`,
  `..._hub_sync_fence_seconds`, `..._hub_durability_unavailable_total`,
  `..._hub_compaction_seconds`, `..._hub_recovery_seconds`,
  `..._hub_disk_bytes`.
- All series carry a bounded `storage_profile={memory,durable}` label so
  dashboards never mix the two profiles' latencies.

### Traces and events

- Spans `hub.commit` → `hub.publish`, plus `hub.persist` / `hub.sync_fence` in
  durable mode, correlated by the `trace_id` carried on the RPC frame.
- Structured events (values excluded): `store_corruption`, `generation_change`,
  `durability_degraded`, `replay_unavailable`, each with `partition`,
  `hub_generation` and bounded sequence fields.

## Verification

- [ ] Metric-name/label tests and failure-path assertions.
- [ ] Dashboards distinguish storage, routing and subscriber failure.
- [ ] Slow/failed telemetry export cannot block partition commit, publication or
  shutdown; bounded queue overflow is observable.
- [ ] Fault injection covers corruption, queue saturation, Sync failure, route
  loss and subscriber backpressure through the Common fault-to-signal matrix.

**Exit:** operators can explain hub durability and propagation health from
bounded signals without debug access to cache values.

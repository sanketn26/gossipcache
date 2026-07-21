# Phase 5: Observability and Reliability Validation

**Goal**: Make cache correctness, invalidation convergence, L2 durability, and production health measurable and actionable.

**Duration**: 2 weeks (Kubernetes-first; MicroVM optional and not on the critical path)

**Prerequisites**: Phases 1–4 complete enough for a running cluster; hybrid milestones **H1–H4** interfaces stable (meters, readiness codes, stream/L2 hooks)

**Status**: Not Started

## Overview

Earlier milestones add instrumentation **hooks**. Phase 5 completes and validates the observability system end to end: metrics, traces, structured events, health semantics, dashboards, SLOs, alerts, and fault-to-signal tests.

The normative signal names, cardinality rules, trace policy, health semantics, and alert categories are defined in the [Hybrid Backed Mode Observability Contract](HYBRID_BACKED_MODE.md#9-observability-contract).

### Ownership (do not double-count with H4)

| Layer | Responsibility |
|---|---|
| H1–H3 | No-op `Meter`/`Tracer`/event interfaces; emit on transitions and RPCs |
| H4 | Minimum safety signals + readiness reason codes + K8s probe wiring |
| **Phase 5** | Full catalog validation, dashboards, SLO burn-rate alerts, runbooks, cardinality CI, overhead budgets, fault-to-signal matrix |

Phase 5 exit gates are release requirements for calling observability “done.” H4 must not claim dashboards/alerts complete.

## Objectives

- [ ] Validate L1 read, transition, fetch-race, singleflight, TTL, eviction instruments
- [ ] Validate TCP/stream ack, replay, gap, queue pressure, reconciliation instruments
- [ ] Validate L2 RPC, version, fencing, journal durability, tier, capacity instruments
- [ ] Export Prometheus metrics through OpenTelemetry-compatible instruments
- [ ] Bounded tracing for misses, writes, replay, anti-entropy, slow/error paths
- [ ] Stable structured operational and audit events
- [ ] Liveness, startup, readiness, authenticated diagnostics
- [ ] **Kubernetes** pod readiness/lifecycle validation (required)
- [ ] MicroVM host/guest profile (optional; see below)
- [ ] Dashboards, recording rules, SLO indicators, alerts
- [ ] Cardinality, overhead, telemetry failure isolation, fault-to-signal coverage

## Architecture Rules

1. Telemetry export must never block L1 reads, invalidation application, or L2 commits.
2. L1-hit instrumentation must be allocation-free and must not create a trace span per hit.
3. Metrics use bounded enum labels. Keys, peer addresses, request IDs, sequences, error strings, and raw node IDs are forbidden as labels.
4. Values, credentials, TLS material, and raw keys never appear in logs or traces.
5. Readiness represents consistency safety, not just process reachability.
6. A collector outage degrades telemetry only; it does not stop cache service or create unbounded memory growth.
7. Platform adapters consume the **same** internal health evaluator and reason codes; they must not invent different safety semantics.

## Implementation Steps

### Step 1: Telemetry Foundation (Day 1–2)

- Confirm internal `Meter`, `Tracer`, and structured-event interfaces with no-op implementations.
- Configure OpenTelemetry resource attributes and asynchronous, bounded exporters.
- Add Prometheus scraping and collector deployment configuration.
- Establish naming, units, histogram boundaries, retention, and redaction policy.
- Add a CI cardinality allowlist for every instrument and label.

**Exit gate**: Components emit telemetry without depending directly on an exporter; exporter failure is non-blocking and memory-bounded.

### Step 2: L1 and State-Machine Signals (Day 2–4)

- Request result and latency for hits, misses, stale hits, failures.
- Resident entries/bytes, evictions, transitions, inflight fetches, retries, singleflight waiters, stale-response rejection.
- Sampled `stale_fetch_rejected` event with epoch/version comparison (no raw key).
- Exemplars for sampled slow misses; no per-hit spans.

**Exit gate**: State/race tests assert expected metric/event; hit-path overhead within budget.

### Step 3: Reliable Invalidation and Anti-Entropy Signals (Day 4–6)

- Connection state, reconnects, ack latency, queue utilization, coalescing, replay volume/lag, sequence gaps, gap age.
- Anti-entropy triggers, duration, keys compared, mismatches.
- Trace replay/reconciliation with span links across fanout.
- Events for gaps, unavailable replay, backpressure, reconciliation lifecycle.

**Exit gate**: Disconnect, replay truncation, queue overflow, and slow-peer tests each produce a distinct signal and correct readiness reason.

### Step 4: L2 and Durability Signals (Day 6–7)

- L2 RPC rate, errors, latency, inflight, `NOT_CAUGHT_UP`.
- Tier utilization, journal lag, fsync latency, compaction, disk capacity, versions, epochs, fencing rejection.
- Always sample version regression, fencing failure, corruption, acknowledged-write durability errors.
- Security/durability audit events to a separate controlled sink.

**Exit gate**: Crash, recovery, failover, corruption, and disk-pressure scenarios identify partition and failure class without high-cardinality labels.

### Step 5: Health and Diagnostics (Day 8)

- Implement `/livez`, `/startupz`, and `/readyz` with structured reason codes.
- Unreconciled gaps, replay overflow, incompatible protocol, incomplete recovery, unavailable required L2 routes, `HUB_GENERATION_MISMATCH`, and **`STREAM_FRESHNESS_UNKNOWN`** affect readiness.
- Origin `StreamCheckpoint` age is part of readiness: a node that stops receiving the tail must not stay ready indefinitely with no gap detected (see hybrid §5.3).
- Authenticated, rate-limited `/debug/cache` and `/debug/peers` (include checkpoint age and applied watermarks).
- Optional **fanout peer** loss alone does not fail readiness while safety and fanout invariants hold. **Origin stream freshness** for every required partition is not optional: missing checkpoints fail readiness with `STREAM_FRESHNESS_UNKNOWN`.

#### Kubernetes readiness profile (required)

- Expose HTTP `/startupz`, `/livez`, and `/readyz` on a dedicated management port.
- `startupProbe` covers journal/index recovery without liveness restarts during expected startup.
- Lightweight `livenessProbe`: event-loop progress only; must not call L2 or require gossip peers.
- `readinessProbe`: protocol compatibility, required L2 routing, replay pressure, reconciliation safety, cluster generation and partition terms installed, origin stream freshness for required partitions.
- On `SIGTERM`: fail readiness first, stop accepting writes, drain inflight RPC and invalidation work within `terminationGracePeriodSeconds`, then exit.
- Authenticated or localhost-only drain/`preStop` endpoint.
- Validate rolling updates, PodDisruptionBudget, topology spread, node drain, HPA bursts, temporary control-plane unavailability.

#### Optional: MicroVM readiness profile

**Not a first-release exit gate.** Implement when MicroVM is a supported deployment target.

- Separate guest boot readiness from cache service readiness.
- Same HTTP probes on the guest management interface; optional compact status over `vsock`. Host treats closed/stale channel as unknown, never ready.
- Bind readiness to MicroVM incarnation ID plus cluster generation and per-partition terms so snapshot restore cannot reuse stale ready state.
- Before snapshot: drain, fail readiness, quiesce writes/telemetry, persist watermarks. After restore: unready until clock, network, identity, L2 routes, epoch, and gaps revalidated.
- Host supervisor bounded boot/readiness deadlines and reason codes (`GUEST_BOOT_INCOMPLETE`, `CLOCK_UNSAFE`, `VOLUME_UNAVAILABLE`, `IDENTITY_MISMATCH`, `NETWORK_UNCONFIGURED`, plus cache-level reasons).
- Validate cold boot, snapshot restore, migration, TAP/vsock interruption, volume remount, clock jump, duplicate identity, partial network.

**Exit gate (required)**: Kubernetes tests distinguish deadlock, incomplete recovery, dependency failure, and consistency-unsafe state without restart loops for ordinary dependency outages.
**Exit gate (optional)**: MicroVM matrix above when the profile is enabled.

### Step 6: Dashboards, SLOs, and Alerts (Day 9)

Ship these dashboards:

1. L1 effectiveness and latency
2. Invalidation delivery, replay, and convergence
3. L2 RPC, storage tiers, and durability
4. Capacity, saturation, and Kubernetes health

Define SLO **indicators** for L1-hit latency, L2 request availability/latency, invalidation convergence, unreconciled-gap duration, and acknowledged-write durability. Establish numeric objectives only after baseline load tests on named hardware against the [reference load profile](HYBRID_BACKED_MODE.md#13-performance-and-load-profile-release-gates).

Page for version regression, fencing failure, corruption, acknowledged-write loss, insufficient ready replicas, and staleness-budget breach. Warning/ticket alerts for growing replay lag, queue saturation, reconnect storms, mismatch rate, tail-latency regression, compaction backlog, and disk forecast.

**Exit gate**: Dashboard and alert rules pass syntax tests; routes, runbook links, ownership, and severity are defined.

### Step 7: Fault-to-Signal and Overhead Validation (Day 10)

- Run the hybrid failure matrix with telemetry assertions.
- Exercise alert rules using recorded or synthetic time series.
- Benchmark telemetry disabled/enabled for L1 hits, misses, invalidations, and L2 commits.
- Simulate collector outage, slow exporter, dropped export batches, and log-sink failure.
- Publish a coverage table mapping each production failure to metric, event, trace decision, readiness effect, alert, dashboard, and runbook.

**Exit gate**: No silent critical failure, no unbounded telemetry queue, no sensitive-data leak, overhead within approved budgets.

## Deliverables

- [ ] Instrumentation library and no-op implementation (if not already from H1)
- [ ] Prometheus/OpenTelemetry configuration
- [ ] L1, invalidation, anti-entropy, and L2 instruments validated
- [ ] Trace sampling and redaction configuration
- [ ] Structured event and audit-event catalog
- [ ] Health/readiness and authenticated diagnostics
- [ ] Kubernetes probe manifests and lifecycle/drain tests
- [ ] (Optional) MicroVM host/guest status adapter and tests
- [ ] Four production dashboards
- [ ] Recording and alert rules with runbooks
- [ ] Cardinality and sensitive-data tests
- [ ] Fault-to-signal coverage report
- [ ] Telemetry overhead benchmark report

## Success Criteria

1. **Coverage**: Every critical failure in the hybrid failure matrix maps to an actionable signal and runbook.
2. **Correctness**: A node with an unreconciled invalidation gap is not ready.
3. **Isolation**: Telemetry failure cannot block or exhaust cache/data paths.
4. **Cardinality**: CI detects unapproved or unbounded labels.
5. **Privacy**: Automated tests find no values, credentials, TLS material, or raw keys in telemetry.
6. **Performance**: L1-hit instrumentation is allocation-free and within the benchmarked overhead budget.
7. **Operations**: Critical alerts have severity, owner, routing, dashboard, and tested runbook links.
8. **Platform readiness**: Kubernetes pod lifecycle cannot advertise ready before cache consistency invariants hold. MicroVM is optional.

## Rollout

1. Deploy metrics and health semantics with alerts disabled.
2. Establish baselines under representative load and failure tests.
3. Tune histogram buckets, sampling, queue limits, and provisional SLOs.
4. Enable warning alerts and validate routing.
5. Enable paging alerts after burn-in and false-positive review.
6. Treat observability acceptance gates as release requirements for subsequent performance optimization (H5).

## Out of Scope

- Business analytics and per-customer usage reporting
- Raw-key search through general-purpose telemetry
- Per-hit distributed tracing
- Automatic remediation beyond Kubernetes probe behavior
- Fixed SLO numbers before representative baseline measurement
- Required MicroVM support for first release

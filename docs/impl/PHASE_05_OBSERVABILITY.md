# Phase 5: Observability and Reliability Validation (hybrid)

**Goal**: Make L1 correctness, invalidation convergence, L2 durability, and production health measurable and actionable.

**Prerequisites**: P0–P4 far enough for a running hub + L1 cluster; **P4** minimum meters, readiness reason codes, and K8s probe wiring stable ([PHASE_PLAN.md](PHASE_PLAN.md) FD-P4.1 / FD-P4.4). Hybrid milestones **H1–H4** in [HYBRID_BACKED_MODE.md](HYBRID_BACKED_MODE.md) map to the same gates.

**Status**: Not Started

**Sequence:** P5 in [PHASE_PLAN.md](PHASE_PLAN.md) (after P4; before P8). Parallel-friendly with P6/P7.

**Normative behavior:** [SEMANTICS.md](../SEMANTICS.md) §9–10  
**Signal catalog / cardinality rules:** [HYBRID_BACKED_MODE.md §9](HYBRID_BACKED_MODE.md#9-observability-contract)  
**Code FDs:** [PHASE_PLAN.md](PHASE_PLAN.md) P5 (FD-P5.1–P5.5)

> Prefer SEMANTICS on conflict. Version authority is **`VersionTag = (partition_id, sequence)`** plus **`hub_generation`** (not Redis versions, not `partition_term` inside the tag). Control plane is **mTLS TCP streams from the hub**, not UDP gossip.

---

## Overview

Earlier phases add instrumentation **hooks** and minimum safety signals. Phase 5 completes and **validates** observability end to end: full metric catalog, traces, structured events, health semantics, dashboards, SLO indicators, alerts, cardinality CI, overhead budgets, and fault-to-signal coverage.

### Ownership (do not double-count with P4 / H4)

| Layer | Responsibility |
|-------|----------------|
| P0–P3 / H1–H3 | No-op `Meter` / `Tracer` / event interfaces; emit on L1 transitions, L2 RPC, stream events |
| P4 / H4 | Minimum safety signals + readiness reason codes + K8s probe wiring |
| **P5 / Phase 5** | Full catalog validation, Prometheus/OTel export, dashboards, SLO burn-rate alerts, runbooks, cardinality CI, overhead budgets, fault-to-signal matrix |

P4/H4 must **not** claim dashboards or paging SLO alerts complete. Phase 5 owns that exit gate.

Maps to PHASE_PLAN:

| FD | Deliverable |
|----|-------------|
| FD-P5.1 | Prometheus exporter + `/metrics` |
| FD-P5.2 | Cardinality allowlist test (CI) |
| FD-P5.3 | OTel tracer hooks on miss / write / replay (not hit) |
| FD-P5.4 | Dashboard JSON + alert rules in `deployments/observability/` |
| FD-P5.5 | Fault-to-signal table test harness |

---

## Objectives

- [ ] Validate L1 read, transition, fetch-race, singleflight, TTL, eviction instruments
- [ ] Validate stream/control-plane: ack, replay, gap, queue pressure, checkpoint age, reconciliation instruments
- [ ] Validate L2 RPC, version sequence, journal durability, tier/capacity instruments
- [ ] Export Prometheus metrics (OpenTelemetry-compatible instruments OK)
- [ ] Bounded tracing for misses, writes, replay, anti-entropy, slow/error paths—not per L1 hit
- [ ] Stable structured operational events (no raw keys/values)
- [ ] `/livez`, `/startupz`, `/readyz` with SEMANTICS reason codes; authenticated diagnostics
- [ ] **Kubernetes** probe and lifecycle validation (required)
- [ ] MicroVM host/guest profile (optional; not first-release gate)
- [ ] Dashboards, recording rules, SLO indicators, alerts
- [ ] Cardinality, overhead, telemetry failure isolation, fault-to-signal coverage

---

## Architecture rules

1. Telemetry export must never block L1 reads, invalidation apply, or L2 commits.
2. L1-hit instrumentation must be allocation-free and must not create a trace span per hit.
3. Metrics use bounded enum labels. Keys, peer addresses, request IDs, sequences, error strings, and raw node IDs are forbidden as labels.
4. Values, credentials, TLS material, and raw keys never appear in logs or traces.
5. Readiness is a **consistency** signal (gaps, stream freshness, hub generation)—not mere process liveness ([SEMANTICS](../SEMANTICS.md) §10).
6. Collector outage degrades telemetry only; no unbounded export queues.
7. Platform adapters use the **same** `internal/health` evaluator and reason codes.

---

## Implementation steps

### Step 1: Telemetry foundation (FD-P5.1, FD-P5.2)

- Confirm `internal/obs` `Meter` / `Tracer` / structured-event interfaces with no-op defaults.
- Async, bounded Prometheus (and optional OTel) exporters; resource attributes from environment.
- Naming, units, histogram boundaries, retention, redaction policy per [HYBRID §9.2](HYBRID_BACKED_MODE.md#92-metrics).
- CI cardinality allowlist for every instrument and label set.

**Exit gate:** Components emit without hard dependency on a live collector; exporter failure is non-blocking and memory-bounded.

### Step 2: L1 and state-machine signals

- Results and latency: hit, miss, stale_hit, error.
- Resident entries/bytes, evictions, transitions, inflight fetches, retries, singleflight waiters, stale-response rejection (`VersionTag` / ceiling—no raw key).
- Sampled `stale_fetch_rejected` event with partition_id + sequence comparison only.
- Exemplars for sampled slow misses; **no per-hit spans**.

**Exit gate:** State/race tests assert expected metrics/events; hit-path overhead within budget.

### Step 3: Invalidation stream and anti-entropy signals

- Connection state, reconnects, ack latency, queue utilization, coalescing, replay volume/lag, sequence gaps, gap age.
- `stream_checkpoint_age`, freshness timeouts → readiness `STREAM_FRESHNESS_UNKNOWN`.
- Anti-entropy: triggers, duration, keys compared, mismatches (held-key path only).
- Trace replay/reconciliation with span links—not one cluster-wide span.
- Events: gaps, unavailable replay, backpressure, reconciliation lifecycle.

**Exit gate:** Disconnect, replay truncation, queue overflow, silent checkpoint stall, and slow-subscriber tests each produce a distinct signal and correct readiness reason.

### Step 4: L2 and durability signals

- L2 RPC rate, errors, latency, inflight, `NOT_CAUGHT_UP`.
- Journal lag, fsync latency, compaction, disk capacity, tier utilization.
- `hub_generation` gauge; per-partition sequence high-water (bounded labels—e.g. `partition_id` only if cardinality capped by config).
- Always sample: version regression, corruption, acknowledged-write durability errors, `HUB_GENERATION_MISMATCH`.

**Exit gate:** Crash, recovery, and disk-pressure scenarios identify failure class without high-cardinality labels.

### Step 5: Health and diagnostics

Implement SEMANTICS §10:

| Endpoint | Meaning |
|----------|---------|
| `/livez` | Event loops alive; **must not** depend on L2 or stream peers |
| `/startupz` | Local recovery and listeners up |
| `/readyz` | Hub routes OK, no unreconciled gaps, **origin stream freshness** for required partitions, `hub_generation` installed, recovery complete |

Reason codes include at least: `RECONCILIATION_REQUIRED`, `STREAM_FRESHNESS_UNKNOWN`, `HUB_UNAVAILABLE` / `L2_ROUTE_UNAVAILABLE`, `HUB_GENERATION_MISMATCH`, `PROTOCOL_INCOMPATIBLE`, `REPLAY_OVERFLOW`.

Optional L1↔L1 fanout peer loss alone ≠ unready. Missing hub checkpoints for a **required** partition ⇒ unready.

Authenticated, rate-limited `/debug/cache` and `/debug/peers` (watermarks, checkpoint age, queue pressure)—see [PHASE_06_SECURITY.md](PHASE_06_SECURITY.md).

#### Kubernetes readiness profile (required)

- Management port probes: `startupProbe` → `/startupz`, `livenessProbe` → `/livez`, `readinessProbe` → `/readyz`.
- On `SIGTERM`: fail readiness → stop writes → drain inflight RPC/stream work → exit within grace period.
- Validate rolling update, PDB, drain, and temporary hub unavailability (not ready, not liveness thrash).

#### Optional: MicroVM readiness profile

**Not a first-release exit gate.**

- Separate guest boot readiness from cache readiness; same HTTP probes; optional vsock status.
- Bind readiness to incarnation ID + `hub_generation` so snapshot restore cannot reuse stale ready.
- Before snapshot: drain, fail readiness, quiesce; after restore: unready until clock, network, identity, L2 routes, and gaps revalidated.
- Host supervisor deadlines and extra reasons (`GUEST_BOOT_INCOMPLETE`, `CLOCK_UNSAFE`, …) plus cache-level reasons.

**Exit gate (required):** K8s tests distinguish deadlock, incomplete recovery, hub loss, and consistency-unsafe state without restart loops for ordinary dependency outages.

### Step 6: Dashboards, SLOs, and alerts (FD-P5.4)

Ship four dashboards:

1. L1 effectiveness and latency  
2. Invalidation delivery, replay, convergence, stream freshness  
3. L2 RPC, storage tiers, durability  
4. Capacity, saturation, Kubernetes health  

SLO **indicators** (numeric objectives only after baseline on named hardware / [reference load profile](HYBRID_BACKED_MODE.md#13-performance-and-load-profile-release-gates)):

- L1 hit latency  
- L2 availability/latency  
- Invalidation convergence  
- Unreconciled-gap duration  
- Acknowledged-write durability  

**Page** on: version regression, corruption, acknowledged-write loss, insufficient ready replicas, staleness-budget breach, persistent `STREAM_FRESHNESS_UNKNOWN` / `RECONCILIATION_REQUIRED`.

**Warn** on: growing replay lag, queue saturation, reconnect storms, anti-entropy mismatch rate, L2 tail latency, compaction backlog, disk forecast.

**Exit gate:** Dashboard and alert rule syntax tests pass; severity, owner, route, runbook links defined.

### Step 7: Fault-to-signal and overhead (FD-P5.5)

- Hybrid failure matrix with telemetry assertions ([HYBRID §11](HYBRID_BACKED_MODE.md#11-required-test-matrix), adjusted to SEMANTICS version model).
- Exercise alerts with recorded/synthetic series.
- Bench telemetry off vs on: L1 hit, miss, invalidation apply, L2 commit.
- Collector outage, slow exporter, dropped batches, log-sink failure.
- Coverage table: failure → metric, event, trace, readiness, alert, dashboard, runbook.

**Exit gate:** No silent critical failure; no unbounded telemetry queue; no sensitive-data leak; overhead within budget.

---

## Deliverables

- [ ] `internal/obs` instrumentation + no-op (if not already from P4)
- [ ] Prometheus (optional OTel) configuration and scrape manifests
- [ ] L1, stream/invalidation, anti-entropy, L2 instruments validated
- [ ] Trace sampling and redaction configuration
- [ ] Structured event catalog (HYBRID §9.4 names)
- [ ] Health/readiness + authenticated diagnostics
- [ ] Kubernetes probe manifests and lifecycle tests
- [ ] (Optional) MicroVM status adapter and tests
- [ ] Four production dashboards under `deployments/observability/`
- [ ] Recording and alert rules with runbooks
- [ ] Cardinality and sensitive-data CI tests
- [ ] Fault-to-signal coverage report
- [ ] Telemetry overhead benchmark report

---

## Success criteria

1. **Coverage:** Every critical hybrid failure maps to an actionable signal and runbook.  
2. **Correctness:** Unreconciled gap or required-stream freshness failure ⇒ not ready.  
3. **Isolation:** Telemetry failure cannot block or exhaust cache/data paths.  
4. **Cardinality:** CI rejects unapproved or unbounded labels.  
5. **Privacy:** No values, credentials, TLS material, or raw keys in telemetry.  
6. **Performance:** L1-hit instrumentation allocation-free within budget.  
7. **Operations:** Critical alerts have severity, owner, routing, dashboard, tested runbook.  
8. **Platform:** K8s lifecycle cannot advertise ready before consistency invariants hold. MicroVM optional.

---

## Rollout

1. Deploy metrics and health semantics with **alerts disabled**.  
2. Baselines under representative load and fault tests.  
3. Tune histograms, sampling, queue limits, provisional SLOs.  
4. Enable warning alerts; validate routing.  
5. Enable paging after burn-in.  
6. Treat Phase 5 gates as release requirements before heavy P8 / H5 performance work.

---

## Out of scope

- Business analytics / per-tenant usage reporting  
- Raw-key search in general-purpose telemetry  
- Per-hit distributed tracing  
- Automatic remediation beyond probe/drain behavior  
- Fixed SLO numbers before measured baselines  
- Required MicroVM support for first release  
- Redis/memberlist/UDP-gossip metrics as the product model  

---

## Related

- [PHASE_PLAN.md](PHASE_PLAN.md) P5 FDs  
- [PHASE_06_SECURITY.md](PHASE_06_SECURITY.md) — P6 mTLS and debug auth  
- [PHASE_07_DEMO_POLISH.md](PHASE_07_DEMO_POLISH.md) — P7 user-facing demo numbers  
- [TESTING_STRATEGY.md](TESTING_STRATEGY.md)  

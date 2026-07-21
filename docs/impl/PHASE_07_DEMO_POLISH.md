# Phase 7: Demo, Repository Polish, and Sponsorship

**Goal**: Package the hybrid L1 + L2 hub product into a clear, demo-friendly open-source experience—runnable locally, honest about consistency, easy to star and sponsor.

**Prerequisites**: **P2–P4** minimum (one L2 hub + ≥2 app/L1 processes with miss/write RPC and invalidation streams). Better after **P5**. See [PHASE_PLAN.md](PHASE_PLAN.md), [SEMANTICS.md](../SEMANTICS.md).

**Status**: Not Started (release polish; not a substitute for core FDs)

**Sequence:** P7 in [PHASE_PLAN.md](PHASE_PLAN.md) (after P2–P4 min; better after P5).

**Normative product story:** [SEMANTICS.md](../SEMANTICS.md), [ARCHITECTURE.md](../ARCHITECTURE.md).

> **Not the Redis + UDP-gossip demo.** No Redis/Valkey as source of truth, no three-node full-value gossip mesh, no “backing store recovery” as the headline path. Prefer SEMANTICS if anything conflicts.

---

## Overview

This phase does **not** add distributed-systems behavior. It packages what hybrid mode already does:

| Prove | How |
|-------|-----|
| Reads stay local | Hit latency on warm L1 |
| Hub is SoT | Write via L2; durable barrier; version tags |
| Peers invalidate, then fetch | After write, other L1s go STALE/EMPTY path → next Get hits L2 |
| W default 0 | Writer OK after hub + local install; peers converge async |
| Ops honesty | `/readyz` reflects gaps / stream freshness |

Positioning line:

> GossipCache is an in-process **L1** cache with a native authoritative **L2 hub**. Hot reads stay local; the hub owns versions and invalidations.

Avoid: “Redis replacement,” “memberlist cache,” “eventual gossip mesh as product.”

---

## Objectives

- [ ] One-command local demo: **1× L2 hub + 2× (or 3×) app+L1**
- [ ] Scenarios: write convergence, update/staleness, delete, hub restart, L1 restart
- [ ] Measured numbers (hardware noted): local hit latency, invalidation→apply lag, write path latency (W=0)
- [ ] Terminal and/or minimal dashboard showing node role, readiness, held keys, stream watermarks
- [ ] Root README rewrite: hybrid shape, when to use / not, quick start, links to SEMANTICS
- [ ] Contribution templates, CI badge, changelog, topics (no Redis-centric topics as primary)
- [ ] Buy Me a Coffee / funding metadata (tasteful)
- [ ] Demo-ready tag (e.g. v0.1.0) only after `make demo` works from a clean checkout

---

## Implementation Steps

### Step 1: Demo contract

**Success criteria:**

- One command starts hub + L1 processes (Compose or Make).
- Distinct ports: L2 RPC, streams, per-process mgmt/metrics (see [DEPLOYMENT.md](../DEPLOYMENT.md)).
- Write key on L1-A → L1-A read-your-writes immediately → L1-B eventually invalidates and refetches.
- Delete propagates (miss / not found after apply + fetch).
- Hub restart: durable value still readable; streams re-subscribe; readiness correct during recovery.
- Demo prints hit/miss, readiness reasons, approximate propagation time—not fake “instant cluster consistency.”

**Suggested command:**

```bash
make demo
```

**Suggested layout:**

```text
demo/
  docker-compose.yml      # l2 + app1 + app2 [+ app3]
  seed.json
  scenarios/
    01-basic-convergence.sh
    02-update-and-staleness.sh
    03-delete-propagation.sh
    04-hub-restart.sh
    05-l1-restart.sh
  README.md
```

Optional binary: `cmd/gossipcache-demo` ([PHASE_PLAN](PHASE_PLAN.md)).

### Step 2: Local environment

Components:

| Service | Role |
|---------|------|
| `l2` | Native hub (journal volume in Compose) |
| `app-1` … `app-N` | Process with embedded L1; `L2_ADDRESSES` → hub |

Quality bar:

- No manual port hunting; documented published ports
- Health wait on hub `/startupz` then L1 `/readyz` before scenarios
- `make demo-down` tears down volumes/networks
- Logs tagged with `node_id` / role (`l2` vs `l1`)
- **No Redis container**

### Step 3: Scenario scripts

**1 — Basic convergence**

1. `Set product:123` via app-1 (or HTTP/debug shim if present).
2. `Get` on app-1 → hit (RYW).
3. `Get` on app-2 before apply → miss or old; after invalidation + fetch → new value.
4. Print apply/propagation timing.

**2 — Update and staleness**

1. Warm key on both L1s.
2. Update via app-1.
3. Show app-2 may serve stale only if `StalePolicy` allows; default `Never` → miss/fetch path.
4. Converge on new `VersionTag`.

**3 — Delete**

1. Create key; delete via app-1.
2. Peers stop serving after invalidation + L2 not-found/tombstone path.

**4 — Hub restart**

1. Write; stop hub; L1 readiness reflects hub loss.
2. Restart hub; recovery; Get returns durable value; streams fresh again.

**5 — L1 restart**

1. Stop app-2; write on app-1; start app-2; subscribe; no silent ready with gap; warm on demand.

### Step 4: Visual demo

Minimum view:

- Processes: hub + L1s, ready/live state, reason codes
- Small key set with local state (`EMPTY` / `VALID` / `STALE` / `FETCHING`) and version sequence
- Hit/miss counters; last invalidation / checkpoint age
- Optional W display (default 0)

Prefer static HTML polling mgmt/debug APIs, or a `watch`+`curl`+`jq` TUI. No heavy frontend toolchain unless already in-repo.

### Step 5: Benchmarks and claims

Cases:

- Local L1 hit (warm)
- L1 miss → L2 Get
- Write W=0 (hub durable + local install)
- Invalidation apply lag under the hybrid reference load profile ([HYBRID](HYBRID_BACKED_MODE.md#13-performance-and-load-profile-release-gates))

Rules:

- Do not claim sub-µs without published benches ([SEMANTICS](../SEMANTICS.md) §12).
- Name hardware, GOMAXPROCS, Compose vs bare metal.
- Raw output under `docs/benchmarks/`; summary in README.

### Step 6: README rewrite

Recommended structure:

```markdown
# GossipCache

In-process L1 + native L2 hub. Caches stay local.

## Why
## When To Use / When Not
## Quick Start
## Demo
## How It Works (L1 / L2 / invalidations / W)
## Status
## Docs (SEMANTICS first)
## Benchmarks
## Contributing
## Support
## License
```

Independent mode and Redis-as-SoT: **not v1**—link SEMANTICS non-goals, do not feature as equal modes.

### Step 7: Repository polish

```text
.github/
  FUNDING.yml
  ISSUE_TEMPLATE/
  PULL_REQUEST_TEMPLATE.md
  workflows/ci.yml
CONTRIBUTING.md
CODE_OF_CONDUCT.md
SECURITY.md
CHANGELOG.md
```

Suggested topics: `go`, `cache`, `distributed-systems`, `eventual-consistency`, `l1-cache`, `kubernetes`—not Redis-first.

### Step 8: Sponsorship

- `.github/FUNDING.yml`
- Short Support section; real handle before publish
- Do not lead the README with sponsorship

### Step 9: Launch assets

- 60–90s recording: `make demo` → write on L1-A → L1-B converges → hub restart recovery
- v0.1.0 notes pointing at SEMANTICS and measured numbers

### Step 10: Release checklist

- [ ] CI green
- [ ] `make demo` from clean checkout
- [ ] Scenarios exit non-zero on failed convergence
- [ ] README quick start verified
- [ ] Benchmarks committed with hardware notes
- [ ] License + changelog
- [ ] No Redis-as-SoT or UDP-gossip claims in user-facing copy

---

## Verification

| Area | Check |
|------|--------|
| Compose stack | Hub + L1s healthy |
| Scenarios | Fail on wrong value or perpetual unready |
| README | Fresh clone works |
| Dashboard/TUI | Reflects invalidation and readiness |
| Claims | Match `docs/benchmarks/` |

---

## Success criteria

- [ ] Visitor understands **L1 + L2 hub** in under a minute
- [ ] Demo in under five minutes without Redis
- [ ] Demo proves local hits and hub-mediated invalidation convergence
- [ ] Staleness / W=0 async peers are explicit
- [ ] Repo hygiene (templates, CI, changelog) present
- [ ] Sponsorship optional and secondary

---

## Anti-goals

- No new core protocol features “for the demo”
- No Redis container as SoT
- No pretending multi-leader independent gossip is v1
- No overclaiming production maturity or latency

---

## Related

- Core FDs: [PHASE_PLAN.md](PHASE_PLAN.md)
- P5 observability release gate: [PHASE_05_OBSERVABILITY.md](PHASE_05_OBSERVABILITY.md)
- P6 security for untrusted nets: [PHASE_06_SECURITY.md](PHASE_06_SECURITY.md)

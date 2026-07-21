# Implementation Status

Single source of truth for **what is built**. Design docs describe the target;
a feature is only real if it appears under **Implemented**.

_Last updated: 2026-07-21_

## Target (v1)

Authoritative product rules: **[SEMANTICS.md](SEMANTICS.md)** — hybrid **L1 +
native L2 hub**. Code-level phases: **[impl/PHASE_PLAN.md](impl/PHASE_PLAN.md)**
(P0–P8).

| In scope | Out of scope for v1 |
|----------|---------------------|
| Embedded L1, native L2 hub as SoT | Redis/Postgres as version authority |
| mTLS TCP invalidation streams from hub | UDP gossip / memberlist control plane |
| VersionTag `(partition_id, sequence)` + `hub_generation` | Independent full-value gossip mode |
| Tunable W (default 0), stale-serve, consistency readiness | Custom RUDP |

## Implemented (local foundation — partial P0)

Useful building blocks; **not** a hybrid cluster yet.

| Area | Location | Notes |
|------|----------|--------|
| Local memory storage | `internal/storage`, `internal/storage/memory` | Sharded map, LRU, TTL, injectable clock |
| Local cache manager | `internal/cache` | Wraps storage; public error translation |
| Public API | `pkg/gossipcache` | `Cache` interface, sentinels, `ServiceRegistry` |
| Library constructor | `pkg/gossipcache/inmemory` | Wires manager + memory engine |
| Config | `internal/config` | Hybrid-shaped defaults (L2 ports, W, freshness); L2 not wired |
| Observability | `internal/observability` | slog + basic L1 Prometheus counters + metrics HTTP service |
| Example | `examples/server` | Local L1 + metrics only (`-tags example`) |
| Benchmarks | `test/benchmark` | Local cache microbench |

## Not started (by phase)

| Phase | Work |
|------:|------|
| P0 remainder | Public facade `New(cfg)`, `VersionTag` types, in-memory L2 fake + basic L1↔backend path |
| P1 | L1 state machine (EMPTY/FETCHING/VALID/STALE), singleflight, apply invalidation |
| P2 | Control plane frames/streams, interest, W confirms |
| P3 | Durable L2 hub (journal, commit, RPC server, `cmd/l2`) |
| P4 | Health/readiness, held-key anti-entropy, K8s manifests, min metrics hooks |
| P5 | Full observability suite |
| P6 | Security (mTLS production path) |
| P7 | Multi-process demo + polish |
| P8 | Performance optims after baselines |

## Removed / non-v1

- **`internal/backingstore` + Redis adapter** — removed; Redis-as-SoT is a SEMANTICS non-goal.
- Config fields for **UDP gossip**, **independent mode**, and **memberlist-era** network ports — removed in favor of L2 hub settings.

Historical ADRs (memberlist, Redis-era evict-on-notify) remain under `docs/adr/` as history; they do not define v1.

## Known debt

- Package layout still uses `internal/cache` + `internal/storage` rather than the target `internal/l1` / `internal/l2` / `internal/control` tree in PHASE_PLAN; reorganize when P0–P1 land.
- Empty `internal/util/` and `test/integration/` placeholders.
- `docs/impl/IMPLEMENTATION_GUIDE.md` is legacy (Redis/gossip walkthrough); non-normative.

Prefer **SEMANTICS** and **PHASE_PLAN** when any older doc conflicts.

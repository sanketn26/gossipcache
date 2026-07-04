# Implementation Status

This is the single source of truth for what is actually built. Design docs
(ARCHITECTURE.md, TECHNICAL_SPEC.md, the `impl/` phase plans) describe the
target system; a feature is only real if it appears in the **Implemented**
section below.

_Last updated: 2026-07-05_

## v1 Scope Contract

v1 is deliberately narrow. The differentiated idea worth proving first is
**backed mode**: local in-memory reads at microsecond latency, with gossip as
a cheap metadata/invalidation channel and the backing store as source of
truth.

**In scope for v1:**

- Backed mode only
- Redis/Valkey as the only backing store
- Static peer discovery only
- Metadata-only gossip (key, version, checksum) + pull-on-mismatch
- Singleflight on backing-store pulls
- A two-node demo with measured numbers: local hit latency, invalidation
  propagation time, bandwidth per write

**Explicitly out of scope for v1** (tracked as v2+/future, not partially
started):

- Independent mode (vector clocks, siblings, custom merge, partition healing)
- Postgres, MySQL, MongoDB, Memcached backing stores
- EC2 / Docker / Kubernetes / DNS discovery
- TLS/mTLS, authentication, rate limiting (Phase 4.5)
- Message compression, LFU eviction, tracing, audit logging, rebalancing,
  backup/restore

## Implemented

- **Local cache** (`internal/cache`, `internal/storage/memory`): sharded
  in-memory storage, LRU eviction, TTL expiration, injectable clock,
  deterministic tests.
- **Public API** (`pkg/gossipcache`): `Cache` interface, error sentinels,
  generic `ServiceRegistry` with ordered start / reverse shutdown,
  `inmemory.New` constructor for library use.
- **Config** (`internal/config`): YAML loading and validation.
- **Observability** (`internal/observability`): slog-based logging,
  Prometheus metrics, metrics service lifecycle.
- **Redis backing store adapter** (`internal/backingstore`,
  `internal/backingstore/redis`): `BackingStore` interface, Redis/Valkey
  adapter with atomic Lua version-bump + TTL, `ErrKeyNotFound` sentinel,
  miniredis-based tests. Not yet wired into the cache manager.

## In Progress

- Backed-mode integration: connecting `internal/cache.Manager` to a
  `BackingStore` (read-through / write-through, version tracking).

## Not Started

- Gossip engine, network transport, membership (see
  [ADR-0001](adr/0001-gossip-transport.md) — decide memberlist vs custom
  before writing any of this)
- Anti-entropy
- Delete tombstones (currently a backing-store delete is indistinguishable
  from "never existed"; must be resolved with the gossip design)
- Interest/held-set semantics: which nodes react to invalidation gossip for
  keys they do not hold (unresolved design question; naive
  pull-on-every-change re-creates the thundering herd at the backing store)
- Benchmarks validating the README performance claims

## Known Debt

- `docs/impl/GAP_ANALYSIS.md` checkmarks describe *plan* coverage, not
  implementation (see the banner in that file).
- Phase docs contain day-level estimates that are not tracked or updated;
  treat them as design references, not schedules.

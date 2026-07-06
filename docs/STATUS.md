# Implementation Status

This is the single source of truth for what is actually built. Design docs
(ARCHITECTURE.md, TECHNICAL_SPEC.md, the `impl/` phase plans) describe the
target system; a feature is only real if it appears in the **Implemented**
section below.

_Last updated: 2026-07-06_

## v1 Scope Contract

v1 is deliberately narrow. The differentiated idea worth proving first is
**backed mode**: local in-memory reads at microsecond latency, with gossip as
a cheap metadata/invalidation channel and the backing store as source of
truth.

**In scope for v1:**

- Backed mode only
- Redis/Valkey as the only backing store
- Static peer discovery only
- Invalidation-only gossip (key, version) with evict-on-notify semantics
  ([ADR-0002](adr/0002-evict-on-notify.md)); pulls happen only on demand
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

- Gossip engine and membership: build as a `hashicorp/memberlist` delegate
  per accepted [ADR-0001](adr/0001-gossip-transport.md); invalidation
  semantics are evict-on-notify per [ADR-0002](adr/0002-evict-on-notify.md).
  No custom transport, wire protocol, connection pool, or peer manager.
- Anti-entropy (evict-based reconciliation per ADR-0002)
- Benchmarks validating the README performance claims (v1 contract numbers:
  local hit latency, invalidation propagation time, bandwidth per write)

## Resolved Design Questions

- **Delete tombstones**: resolved by [ADR-0002](adr/0002-evict-on-notify.md).
  A delete broadcasts the same invalidation as a write; the backing store is
  the source of truth, so `ErrKeyNotFound` on re-pull is an ordinary miss and
  backed mode needs no tombstones.
- **Interest/held-set semantics**: resolved by
  [ADR-0002](adr/0002-evict-on-notify.md). Nodes evict on notification instead
  of pulling, so nodes that do not hold a key do nothing and the
  thundering-herd problem never arises.

## Known Debt

- `docs/impl/GAP_ANALYSIS.md` checkmarks describe *plan* coverage, not
  implementation (see the banner in that file).
- Phase docs contain day-level estimates that are not tracked or updated;
  treat them as design references, not schedules.

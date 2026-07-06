# ADR-0002: Backed-Mode Invalidation Is Evict-on-Notify

**Status**: Accepted
**Date**: 2026-07-06

## Context

The Phase 2 plan's gossip semantics are **pull-on-notify**: when a node
receives a change notification, it compares checksums and pulls the key from
the backing store — including for keys it has never cached. That one choice
creates a cluster of problems:

- **Thundering herd**: every write triggers a backing-store read from every
  node in the cluster, defeating the bandwidth/load claims of backed mode.
  Fixing it requires "interest"/held-set tracking, an unresolved design
  question.
- **Singleflight bypass**: the plan's singleflight guards only the read-miss
  path; gossip-triggered pulls — where the herd actually forms — go around it.
- **Non-atomic (version, checksum) pairs**: the version comes from the Redis
  Lua script, but the checksum is computed client-side. Under concurrent
  writers a notification can carry a pair that never existed in the store.
- **Local version metadata**: version/checksum comparison requires extending
  local storage entries with version metadata before gossip can work.
- **No delete story**: there is no delete notification or tombstone design for
  backed mode; a deleted key is served stale until TTL expiry.

## Decision

When a node receives an invalidation for a key, it **deletes its local copy
and does nothing else**. Data moves only through the demand path: the next
local read misses, and pulls from the backing store through singleflight.

Protocol consequences:

- The invalidation message is `key + version + origin node ID`. **No
  checksum.**
- A node that does not hold the key does nothing (evicting a missing key is a
  no-op). No interest tracking is needed.
- **Delete is the same invalidation message.** No tombstones in backed mode:
  the backing store is the source of truth, and `ErrKeyNotFound` on the next
  pull is an ordinary miss.
- `version` is a deduplication optimization only (e.g., the writer ignoring
  its own broadcast). Correctness never depends on it, because evicting is
  always safe — the worst case is one redundant re-fetch.
- Anti-entropy follows the same rule: reconciliation identifies stale or
  deleted local entries and **evicts** them; it never bulk-pulls.

## Consequences

**Positive:**

- Thundering herd is gone by construction; backing-store load after a write is
  proportional to actual read demand, and every pull is singleflighted.
- Delete propagation is solved with zero extra protocol.
- The message is a few dozen bytes — comfortably inside memberlist's ~1400-byte
  UDP broadcast budget ([ADR-0001](0001-gossip-transport.md)), even with large
  keys.
- No client-side checksum, so the atomicity problem disappears.
- Local storage does not need version metadata for v1.
- The memberlist delegate becomes trivial: broadcast invalidations, evict on
  receipt.

**Negative / accepted trade-offs:**

- The first read after a write pays one backing-store round-trip; entries are
  never pre-warmed. This is the classic invalidate-vs-update cache decision;
  invalidate is the standard answer for exactly the reasons above.
- Slightly more read traffic to the backing store for genuinely hot keys that
  are written often. Acceptable for v1.

**Revisit if:** measured post-invalidation read latency on hot keys matters in
practice. Options then: optional pre-warm for a configured hot set, or
piggybacking small values on invalidations (v2 territory, overlaps with
independent mode's full-data gossip).

## Effect on existing plans

In `docs/impl/PHASE_2_BACKED_MODE.md`:

- Step 3's `Checksum` field and checksum serialization are removed from the
  message design.
- Step 5's `handleChangeNotification`/`needsPull`/`pullFromBackingStore`
  handlers are superseded: the handler is "evict local entry, done".
- Step 6's `BackedCache` read/write paths remain valid (read-through with
  singleflight, write-through then broadcast), but the broadcast carries no
  value/checksum and delete broadcasts the same invalidation.
- Anti-entropy (Step 5.5) remains, reinterpreted as evict-based
  reconciliation.

Together with ADR-0001, this resolves the "Delete tombstones" and
"Interest/held-set semantics" open questions in `docs/STATUS.md`.

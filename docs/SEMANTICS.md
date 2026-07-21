# GossipCache Semantics and Design Choices

**Status:** Locked for v1 product direction (hybrid L1 + L2 hub)
**Role:** Single place for *what* the system means and *which* choices we made.
**Implementation detail:** [impl/HYBRID_BACKED_MODE.md](impl/HYBRID_BACKED_MODE.md)
**APIs/types sketch:** [TECHNICAL_SPEC.md](TECHNICAL_SPEC.md)

Independent / Redis-as-SoT / UDP-gossip modes are **out of scope for v1**.

---

## 1. Product wedge

| Choice | Decision |
|--------|----------|
| Product | In-process **L1** cache + native authoritative **L2 hub** |
| Philosophy | Caches must be local; network is for miss/write and control, not every hit |
| Not v1 | Independent full-value gossip mesh; Redis/Postgres as version authority; custom RUDP |
| L2 shape | **One logical hub** (may be HA as a unit). Internal **partitions/shards** scale journals and invalidation streams—not “each app owns one partition” and not multi-leader politics exposed to L1 |

---

## 2. Planes

| Plane | Carries | Path |
|-------|---------|------|
| Data | Values | Hit: L1 memory. Miss/write: L1 ↔ L2 RPC |
| Control | Versioned invalidations only | Hub changefeed → interested L1s (mTLS TCP) |

Values never ride the control plane.

---

## 3. Version tag

```text
VersionTag = (partition_id, sequence)
```

This is the **only** authoritative per-key version format (SEMANTICS wins over any older hybrid drafts that mentioned `partition_term` inside the tag).

| Field | Meaning |
|-------|---------|
| `partition_id` | Hub-internal shard for the key; part of the tag so versions are not compared across shards |
| `sequence` | **Strictly monotonic** commit order for that shard for the life of the current **hub_generation** |

**Alongside the tag (not inside it):**

| Field | Meaning |
|-------|---------|
| **hub_generation** | Config / recovery generation for the logical hub. Validated at connect and readiness. Bumped only when the hub cannot safely continue prior sequence spaces (e.g. restore without contiguous log). |

**Ordering:** compare only when `partition_id` is equal: higher `sequence` is newer. **Sequences must not reset** while `hub_generation` is unchanged. If recovery cannot continue sequences, advance `hub_generation` and force L1 revalidation—do not invent a per-key `partition_term`.

**Not in the tag:** stream delivery sequence, subscriber set, node id, wall clock, W, partition_term.

**Every durable hub mutation** must atomically:

1. Persist value or tombstone
2. Assign next `VersionTag` (candidate seq, then commit)
3. Append changefeed/invalidation event with next `stream_sequence` for that shard’s stream

No silent value-only commits.

---

## 4. Partitions and interest (who gets work)

Partitions exist so the **hub is not a single overwhelmed bus**, not so each app process is pinned to one shard.

An app+L1 may read/write **many** partitions.

| Layer | Who is touched on write of key K ∈ shard P |
|-------|-----------------------------------------------|
| Hub | Shard P only |
| Stream | Subscribers of P’s stream (not every process by default) |
| Key state machine | Only L1s that **hold K** or are **fetching K** (else watermark only) |

- Subscribe on first interest in P (or subscribe-all if shard count is small).
- **v1:** do not unsubscribe-when-empty (churn/races not worth it).
- Peer L1 fanout is optional; preferred scale path is **hub → subscribers**.

---

## 5. L1 key states

| State | Queryable payload? | Meaning |
|-------|--------------------|---------|
| `EMPTY` | no | No resident record |
| `FETCHING` | no* | Singleflight to L2 |
| `VALID` | yes | Local copy at known `VersionTag` |
| `STALE` | policy | Ceiling > local version; may retain bytes for stale-serve |

\*Unless stale-serve policy returns retained bytes on error / SWR.

**Stale-serve policy** (default `Never`): `Never` | `StaleIfError` | `ServeStaleWhileRevalidate`.

---

## 6. Read path

```text
Get(K)
  VALID + not expired → return local
  else → singleflight L2 Get(min_version?)
       → install VALID only if VersionTag ≥ invalidation ceiling
          and partition matches
```

Invalidation during fetch raises ceiling; do not cancel waiters; reject stale L2 responses (`NOT_CAUGHT_UP` / below ceiling → retry).

---

## 7. Write path

```text
Set/Delete(K)
  1. L1 → L2 RPC
  2. Hub atomic barrier: value + VersionTag + stream event
  3. Writing L1 installs local VALID/tombstone from RPC response  ← RYW this process
  4. Return OK per W (see §8)
  5. Hub pushes invalidation to stream subscribers
  6. Peers: apply → usually STALE → next Get fetches L2
  7. Writer seeing own invalidation at same version: idempotent
```

Writer does **not** mint stream sequences. Hub is sole origin of invalidations.

---

## 8. Visibility and tunable W

**Visibility point:** hub durable commit.

| When | What readers may see |
|------|----------------------|
| Write in flight | Old value everywhere (including writer) — allowed |
| After OK, **writer** (W any) | New value (local install before OK) |
| After OK, **other processes**, **W = 0** | May still be old until invalidation + apply (+ fetch) |
| After OK, **W = k** | At least k other L1s confirmed per confirm level |

### Tunable write acknowledgements **W** (not a separate mode)

Same pipeline; **W** only sizes the peer-confirm wait. **Architecture (locked):** **hub-aggregated W**.

| W | Behavior | Cost |
|---|----------|------|
| **0** (default) | OK after hub durable + local install | Normal write latency |
| **k > 0** | Client includes `W` on Set/Delete RPC; **hub** does not complete the RPC until **k distinct** peer nodes have confirmed `InvalidateApplied` for that write’s stream event (or timeout) | Latency, availability coupling |

**Why hub-aggregated:** avoids the race where a peer confirms before the writer registers a local waiter. The Set RPC is the correlation; confirmations route to the hub, which finishes the RPC.

**Confirm rules:**

- Level v1: `InvalidateApplied` (not hop-only; not full value fetch).
- Deduplicate by **confirming node id** — one peer counts at most once toward W.
- Writer node does not count toward W.
- On timeout: RPC returns `ErrWriteConfirmTimeout`; **hub commit already durable**; no rollback; peers still converge async.

Raising W is **optional and costly**. Use sparingly.

---

## 9. Control-plane delivery

| Mechanism | Meaning |
|-----------|---------|
| Transport | mTLS TCP; UDP not used for invalidations |
| Hop ack | Bytes received — not enough to drop hub replay |
| Application ack / confirm | Applied to key SM (or durable relay log recovered before ready) |
| Gap | Replay from hub; expired → `RECONCILIATION_REQUIRED` + anti-entropy before ready |
| `StreamCheckpoint` | Idle heartbeat; age > timeout → `STREAM_FRESHNESS_UNKNOWN` (unready) |
| Anti-entropy | Held keys only vs hub summaries |

---

## 10. Readiness

| Endpoint | Meaning |
|----------|---------|
| `/livez` | Process event loops alive |
| `/startupz` | Local recovery done |
| `/readyz` | Safe to serve: hub routes OK, no unreconciled gaps, stream freshness for **required** subscriptions, generation installed |

Optional fanout **peer** loss ≠ unready. Missing **hub** stream freshness for a required shard ⇒ unready.

Primary ops profile: **Kubernetes**. MicroVM optional later.

---

## 11. Failure summary

| Failure | Behavior |
|---------|----------|
| Hub down | Writes fail closed; misses fail or stale-serve policy |
| Stream gap past retention | Not ready until reconcile |
| Silent stall (no checkpoints) | `STREAM_FRESHNESS_UNKNOWN` |
| Hub HA | Treat as **hub** failover/replication (internal shards stay hub-owned). Not app-visible multi-leader elections |

---

## 12. Non-goals (v1)

- Redis/Postgres as version authority
- Custom RUDP
- L1-originated invalidation publish
- Full-value peer gossip
- Cross-pod RYW without raising W
- Unsubscribe-on-empty subscription churn
- Independent mode as product focus
- Claiming sub-µs without published benchmarks

---

## 13. Decisions log (condensed)

| # | Choice |
|---|--------|
| 1 | Hybrid L1 + native L2 hub is the product |
| 2 | Hub is logical SoT; partitions scale the hub internally |
| 3 | Version tag includes `partition_id` |
| 4 | Every hub write advances version + stream |
| 5 | App may use many partitions; interest + held-key apply |
| 6 | W = 0 default; higher W = tunable peer confirms, costly |
| 7 | Read-your-writes on writer via RPC response install |
| 8 | In-flight and other pods may be stale until hub commit (+ stream) |
| 9 | App ack ≠ hop ack; stream freshness required for ready |
| 10 | Independent / Redis-SoT / UDP control plane out of v1 |

Detail and wire sketches: [impl/HYBRID_BACKED_MODE.md](impl/HYBRID_BACKED_MODE.md).

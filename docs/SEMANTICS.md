# GossipCache Semantics and Design Choices

**Status:** Locked for v1 product direction (hybrid L1 + L2 hub)
**Role:** Single place for *what* the system means and *which* choices we made.
**Implementation phases:** [impl/README.md](impl/README.md)
**APIs/types sketch:** [TECHNICAL_SPEC.md](TECHNICAL_SPEC.md)

Independent / Redis-as-SoT / UDP-gossip modes are **out of scope for v1**.

---

## 1. Product wedge

| Choice | Decision |
|--------|----------|
| Product | In-process **L1** cache + native memory-first **L2 hub** |
| Philosophy | Caches must be local; network is for miss/write and control, not every hit |
| Not v1 | Independent full-value gossip mesh; Redis/Postgres as version authority; custom RUDP |
| L2 shape | **One logical hub**. Internal **partitions/shards** scale the memory table and invalidation streams—not “each app owns one partition” |
| Default storage | **Memory**: Hub state is intentionally ephemeral; restart starts empty under a new `hub_generation` |
| Opt-in storage | **Durable**: synchronous persistence may preserve acknowledged mutations across restart, at additional latency and operational cost |

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
| **hub_generation** | Logical Hub incarnation for sequence-space validity. Compared for equality at connect/readiness. Memory mode creates a fresh generation every start; durable mode preserves it only while sequence continuity is recovered safely. |

**Ordering:** compare only when `partition_id` and `hub_generation` match: higher
`sequence` is newer. Generations themselves are identity values, not ordered
counters. **Sequences must not reset** while `hub_generation` is unchanged. A
memory restart or unsafe durable recovery creates a different generation and
forces L1 revalidation—do not invent a per-key `partition_term`.

**Not in the tag:** stream delivery sequence, subscriber set, node id, wall clock, W, partition_term.

**Every Hub mutation** prepares one candidate containing:

1. Value or tombstone plus expiry
2. Next candidate `VersionTag`
3. Changefeed/invalidation event with the next candidate `stream_sequence`

The candidate becomes visible atomically in the Hub memory table, version head
and stream. **Publish order depends on acknowledgement mode** (do not invert):

- **WriteFast** (default): on a durable-profile Hub, accept the complete record
  into the bounded ordered persistence queue first; then atomically publish in
  memory and acknowledge. Acknowledgement does **not** wait for storage sync.
  Restart survival is not promised for Fast-acked writes.
- **WriteSync**: synchronously persist this candidate **and every earlier
  partition mutation** (fence), **then** atomically publish in memory and
  acknowledge. Memory must not become visible before the fence succeeds.

A rejected enqueue or Sync fence publishes nothing and consumes no version or
stream sequence. No profile permits a silent value-only commit or a
memory-then-fsync Sync path.

### Storage profiles

| Profile | Default | Success means | Restart behavior |
|---------|---------|---------------|------------------|
| `memory` | **Yes** | Value, version and stream event committed in Hub memory | State is lost; Hub starts empty with a new `hub_generation`; Nodes discard/revalidate old-generation entries |
| `durable` | No | Hub has a persistence backend; the per-write acknowledgement mode selects memory-fast or synchronous success | Sync-acknowledged mutations recover; fast writes may be lost on an unclean restart |

The profile is fixed for a Hub data directory/process lifetime and advertised in
the connection handshake. Switching profile requires a controlled restart. The
Node API and control protocol remain the same; durability is a Hub acknowledgement
property, not a second cache-consistency protocol.

### Write acknowledgement modes

| Mode | Default | Requirement | RPC success means |
|------|---------|-------------|-------------------|
| `WriteFast` | **Yes** | Any Hub profile | Atomic memory commit completed; invalidation is eligible. A durable-capable Hub queues ordered persistence, but restart survival is not promised. |
| `WriteSync` | No | Durable profile with healthy persistence | This mutation **and every earlier partition mutation** crossed the synchronous durability boundary before success. |

`WriteSync` against a memory-only or unhealthy persistence backend returns
`ErrDurabilityUnavailable` without committing the mutation. A Sync write is a
partition persistence fence: it flushes earlier queued Fast writes before its
own record so recovered version and stream sequences cannot contain holes.

Fast and Sync affect restart durability only. **W is orthogonal**: it controls
peer invalidation confirmation. A request can therefore be Fast/W=0,
Fast/W=k, Sync/W=0, or Sync/W=k.

After an unclean durable-profile restart where acknowledged Fast writes may have
been lost, the Hub creates a different `hub_generation` even if it recovers a
valid persisted prefix. Nodes must revalidate against that new generation.

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
- v1 delivery is direct **Hub → subscriber** streaming. Subscriber-to-subscriber
  relay/fanout requires a later protocol contract.

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
  2. Hub commit barrier per WriteFast/WriteSync: value + VersionTag + stream event
  3. Writing L1 installs local VALID/tombstone from RPC response  ← RYW this process
  4. Return OK per W (see §8)
  5. Hub pushes invalidation to stream subscribers
  6. Peers: apply → usually STALE → next Get fetches L2
  7. Writer seeing own invalidation at same version: idempotent
```

Writer does **not** mint stream sequences. Hub is sole origin of invalidations.

---

## 8. Visibility and tunable W

**Visibility point:** successful Hub memory commit; for `WriteSync`, persistence
must complete before that mutation becomes visible.

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
| **0** (default) | OK after selected Fast/Sync commit + local install | Fast avoids storage wait; Sync includes persistence fence latency |
| **k > 0** | Client includes `W` on Set/Delete RPC; **hub** does not complete the RPC until **k distinct** peer nodes have confirmed `InvalidateApplied` for that write’s stream event (or timeout) | Latency, availability coupling |

**Why hub-aggregated:** avoids the race where a peer confirms before the writer registers a local waiter. The Set RPC is the correlation; confirmations route to the hub, which finishes the RPC.

**Confirm rules:**

- Level v1: `InvalidateApplied` (not hop-only; not full value fetch).
- Deduplicate by **confirming node id** — one peer counts at most once toward W.
- Writer node does not count toward W.
- On timeout: RPC returns `ErrWriteConfirmTimeout`; the selected **Fast/Sync commit already succeeded**; no rollback; peers still converge async. Only Sync implies the documented restart durability.

Raising W is **optional and costly**. Use sparingly.

---

## 9. Control-plane delivery

| Mechanism | Meaning |
|-----------|---------|
| Transport | mTLS TCP; UDP not used for invalidations |
| Hop ack | Bytes received — not enough to drop hub replay |
| Application ack / confirm | Applied to the direct subscriber's key state machine |
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

A direct subscriber losing required **Hub** stream freshness is unready until
reconnect/replay or reconciliation restores consistency evidence.

Primary ops profile: **Kubernetes**. MicroVM optional later.

---

## 11. Failure summary

| Failure | Behavior |
|---------|----------|
| Hub down | Writes fail closed; misses fail or stale-serve policy |
| Memory Hub restart | All Hub values are lost; new `hub_generation`; Nodes invalidate/revalidate old-generation state before ready |
| Durable Hub restart (clean) | Restore Sync-acknowledged mutations and Fast mutations known to have crossed the persistence boundary; sequences stay contiguous under the same generation when recovery is safe |
| Durable Hub restart (unclean Fast tail) | Recover a valid persisted prefix only; create a new `hub_generation` so Nodes revalidate; recovery failure is not ready and fails closed |
| Stream gap past retention | Not ready until reconcile |
| Silent stall (no checkpoints) | `STREAM_FRESHNESS_UNKNOWN` |
| Hub process/volume failure | v1 fails closed; automatic multi-replica replication, leader fencing and failover are post-v1. Any later HA remains internal to one logical Hub, not app-visible multi-leader elections. |

---

## 12. Non-goals (v1)

- Redis/Postgres as version authority
- Custom RUDP
- L1-originated invalidation publish
- Full-value peer gossip
- Subscriber-to-subscriber relay / pure peer fanout (v1 is Hub → direct subscribers only)
- Separate subscription leases (connection state + checkpoints gate freshness)
- Multi-replica Hub HA / automatic failover (post-v1; v1 is one logical Hub process)
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

Implementation contracts and work: [impl/README.md](impl/README.md).

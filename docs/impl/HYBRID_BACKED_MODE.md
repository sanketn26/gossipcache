# Hybrid Backed Mode Implementation Plan

**Status**: Implementation detail for v1 hybrid

**Locked product semantics:** [../SEMANTICS.md](../SEMANTICS.md). If this file disagrees with SEMANTICS, **SEMANTICS wins**.

**Scope**: L1 + L2 hub. Independent mode / Redis-as-SoT are out of v1 focus.

**Goal**: Wire formats, state-machine tables, stream algorithms, milestones, and test matrix that implement SEMANTICS.

## 1. Decisions and Corrections

> Prefer the condensed log in [SEMANTICS §13](../SEMANTICS.md#13-decisions-log-condensed). Items below are historical/implementer notes.

1. **Reliable TCP is the control-plane baseline.** Invalidation frames use persistent, mutually authenticated TCP with application-level acknowledgement, replay, and gap detection. UDP is not used for backed-mode invalidations. Custom reliable-UDP is out of scope. QUIC may sit behind the same transport interface later.
2. **Transport reliability is not application delivery.** TCP reconnects, process crashes, and bounded queues can still lose events. Every event has a stream sequence; peers retain a replay window; expired gaps force anti-entropy before readiness.
3. **The magic byte is `0xC7`.** The proposed `0xHC` is not valid hexadecimal.
4. **Sub-microsecond reads are a benchmark objective, not a guarantee.** They apply only to an L1 hit with no serialization, allocation, lock contention, or network access. Release gates use measured percentiles on named hardware and a named load profile (see §13).
5. **L2 must be highly available before production.** A single L2 instance is development-only. Production uses partition ownership, replicas, durable version state, and fencing.
6. **Versions are `(partition_id, sequence)` only** (SEMANTICS). No `partition_term` inside the tag. Sequences are strictly monotonic per partition under a **hub_generation**; they do not reset without a hub_generation bump that forces L1 revalidation. Partitions scale the logical hub—not app-visible multi-leader terms.
7. **L2 is the sole invalidation publisher (v1).** Partition leaders assign stream sequences and emit invalidations after durable commit. Writing L1s do not publish independently. L1 peers only fan out and acknowledge; they never renumber events.
8. **Application stream acks mean applied, not merely buffered.** `StreamAcknowledgement.contiguous_stream_sequence` advances only after the event has been applied to the L1 state machine (or, for origin/relay replay logs, after durable persistence that is recovered before readiness). In-memory-only retention must not advance the application watermark.
9. **Stream freshness is required for readiness.** Detected gaps are necessary but not sufficient: a silent stall (no further sequences) must fail readiness via origin checkpoint/heartbeat freshness. See §5.3.
10. **Redis/Postgres external “backing stores” are not the backed-mode source of truth.** Native L2 is. Optional adapters that sync L2 to an external system are future work and out of scope for the first release.
11. **Visibility is hub-centric.** Other nodes learn of updates only after L2 durable commit, via the hub invalidation stream (direct subscribe or fanout of hub events)—not a peer value push. Until commit, concurrent and in-flight reads may be stale, including on the writing process.
12. **Every durable L2 mutation advances version propagation.** A write must not become committed/acked unless it atomically assigns the next **version tag** `(partition_id, sequence)`, appends the changefeed event with the next `stream_sequence`, and makes that event eligible for stream publish. Silent commits without version propagation are forbidden.
13. **Do not refresh every node for every key.** Invalidations are **partition-scoped** and delivered only to L1s that **subscribe** to that partition (interest). Application to the key state machine is **held-key only**. Full-mesh “notify all processes of all keys” is not the design.
14. **Write visibility acknowledgements are tunable concurrency (W), not a separate product.** Default is low-W (hub durable + local install; peers catch up async). Higher W waits for more peer invalidation confirms before OK—**optional, costly, operator/API tunable**. Same write path; only the wait set size changes.

## 2. Target Architecture

```text
Application pod A                         Application pod B
┌──────────────────────┐                 ┌──────────────────────┐
│ app + embedded L1    │◄───────────────►│ app + embedded L1    │
│ per-key state machine│  reliable TCP   │ per-key state machine│
└──────────┬───────────┘  fanout of L2   └──────────┬───────────┘
           │ RPC miss/write   streams                │
           └──────────────────┬──────────────────────┘
                              ▼
              ┌────────────────────────────────┐
              │ authoritative L2 hub           │
              │ partitioned ownership          │
              │ RAM index + hot payload tier   │
              │ NVMe warm/cold payload tier    │
              │ durable epoch/version journal  │
              │ changefeed = invalidation src  │
              └────────────────────────────────┘
```

L1 is a library linked into the application, not a sidecar. L2 is the source of truth. A write commits to L2 before its invalidation is eligible for publication. Gossip/fanout carries keys and versions only; it never carries values.

### 2.0 L2 write ⇒ version + propagation (normative)

On L2, **data write and version propagation are one commit**, not two best-effort steps.

```text
L2 mutate (Set / Delete / CAS / admin repair / compaction that changes visible value)
  ──atomic durable unit──►
      1. new payload or tombstone
      2. version tag (partition_id, sequence++)   // data order; seq strictly monotonic
      3. changefeed event + stream_sequence++     // control-plane order
  ──then──►
      4. ack client RPC (if any)
      5. publish InvalidationBatch only to subscribers of that partition stream
      6. StreamCheckpoint head advances for that stream (includes this seq)
```

| Rule | Requirement |
|---|---|
| No silent commit | If the key’s committed value/tombstone changes, a new record version **and** a stream event **must** exist in the same durability barrier. |
| One path | All mutation entry points share this barrier (L1 RPC, future admin API, restore that exposes new values). No “write value only, invalidate later.” |
| Ack ordering | Client write ack only after steps 1–3 are durable. Propagation (5) may be async after that, but the event must already be in the durable replay/changefeed so crash recovery still publishes it. |
| Partition-scoped publish | Event for key in partition `P` goes on `stream_id = (P)` only—not a global “all keys” bus. |
| Subscriber-scoped delivery | L2 pushes to **current subscribers of P**, not to every L1 in the cluster by default. |
| Idempotent re-publish | After restart, L2 may re-send unacked stream ranges; receivers dedupe by `(stream_id, stream_sequence)` or key+version. |
| Checkpoints | `head_stream_sequence` reflects the highest published mutation seq for **that** stream so L1 freshness tracks write activity on partitions it cares about. |

**Failure if violated:** Subscribers that hold the key keep serving old `VALID` data with no invalidation—silent staleness for interested nodes.

### 2.1 Visibility and staleness (normative)

Backed mode is **not** linearizable across processes. The single ordering and durability point is **L2 durable commit**. Invalidation fanout is how non-writers converge on that hub truth; it is not a second source of truth.

| Observer | Before L2 commit completes | After L2 commit, before local L1 install / apply | After local apply / install |
|---|---|---|---|
| **Writing L1** | Gets may return the pre-write value (in-flight Set does not update L1 yet). Concurrent Gets and singleflight waiters can still be stale. | Must install from the Set/Delete RPC response before returning OK (§5.1). After that, Gets on this process see the write. | Read-your-writes for that process. |
| **Other L1s** | Unaffected by the in-flight write. Local `VALID` hits remain pre-write; L2 Get may still return the old version if commit has not finished. | Learn **only** via hub invalidation stream (subscribe or fanout of hub events)—same mechanism as any other change detected at the center. Apply → typically `STALE`, then refetch on read. | See new value after refetch (or degraded stale per policy while refreshing). |
| **L2** | Write not durable; not in changefeed; not visible to Gets that observe committed state. | Authoritative new version; changefeed eligible for publish. | — |

**Implications:**

1. **Other-node updates ≡ central hub propagation.** There is no special “remote L1 told me” data path. A peer’s Set becomes visible elsewhere only after **that** Set commits on L2 and invalidations (and subsequent Gets) catch up. Same as any hub-mediated cache.
2. **In-flight means stale is allowed.** Until L2 acks the mutation, no reader—writer process included—is required to observe the new value. Applications that need “I have started Set” visibility must wait for Set’s OK (and even then only the writing process is guaranteed without further sync).
3. **Cross-process read-your-writes is not the default.** After a normal (async) Set returns OK, another pod may still serve old L1 data until invalidation is applied and it refetches. That lag is expected for the default path.
4. **Tunable W** can wait for peer confirmations before OK (§2.3); default W = 0. Raising W is optional and expensive—not a separate system.
5. **Singleflight does not freeze a snapshot of “intent to write.”** A Get that is `FETCHING` while a Set is in flight may complete with a pre-commit L2 value; if the Set commits first, the usual ceiling / minimum_version rules apply when responses race. The writing process must still install the Set response so post-OK Gets are fresh locally.
6. **Stale-serve policies** only widen when stale data may be returned deliberately; they do not create early visibility of uncommitted writes.

```text
timeline (one key)

  t0  peer B Get → L1 VALID (old) or L2 old
  t1  peer A Set in flight ──────────────► L2 (not committed)
      A Get / B Get may still be old          ↑ in-flight = stale OK
  t2  L2 durable commit + changefeed
  t3  A installs VALID (new) → A Set returns OK → A Get = new
  t4  invalidation reaches B → STALE
  t5  B Get → L2 Get → VALID (new)
```

### 2.3 Tunable write acknowledgements (W)

This is **tunable concurrency / visibility**, same idea as a write quorum **W**—not a second architecture.

One write pipeline; **W** only changes how many peers must confirm before the call returns.

```text
always:  L2 durable barrier + writing L1 local install
then:    wait for up to W peer confirms (W = 0 means don't wait)
```

| W | Name | When Set returns OK | Typical use |
|---:|---|---|---|
| **0** | default | Hub durable + local install only | Hot path; max write concurrency / min latency |
| **1..k** | raised | Plus **k** distinct peer confirms | “At least some other nodes have applied invalidation” |
| **high** | expensive | Large peer set | Rare; availability and latency suffer |

**W = 0 is the default.** Raising W is opt-in and **costly** (latency, peer availability, control-plane wait). It does not change L2 versioning or who publishes invalidations.

#### What a “confirm” counts as (ConfirmLevel)

| Level | Peer did | If OK, those peers… | Cost |
|---|---|---|---|
| **`InvalidateApplied`** (v1) | Applied hub invalidation (ceiling / `STALE` if held; not hop-only) | Will not keep pre-write as **`VALID`** if they held the key | Medium |
| **`ValueVisible`** (later) | Hold `VALID` at new version (fetched) | Plain Get returns new value | High |

v1: implement **W + InvalidateApplied** only.

#### How W is tuned

| Knob | Role |
|---|---|
| **`W` / `ConfirmCount`** | How many **other** ready L1s must confirm (0 = default async return) |
| **`ConfirmLevel`** | Depth of confirm (invalidate vs value visible) |
| **`WriteTimeout`** | Max wait for the W set; on timeout → error by default, **no L2 rollback** |
| **Targeting** | e.g. any ready subscribers of that partition stream / sample—not “all nodes” unless W is set absurdly high |

```go
type WriteOptions struct {
    W              int           // 0 default; tunable concurrency of peer acks
    ConfirmLevel   ConfirmLevel  // InvalidateApplied
    WriteTimeout   time.Duration // only meaningful when W > 0
}

Set(ctx, key, val, ttl)                          // W=0
SetWithOptions(ctx, key, val, ttl, WriteOptions{W: 1, WriteTimeout: 100 * time.Millisecond})
```

Cluster or key-class defaults can set a floor/ceiling for W; per-call options override.

#### Cost of raising W (why keep default 0)

| Effect | Why |
|---|---|
| Lower write concurrency / higher latency | Call blocks on the W-th confirmer |
| Availability coupling | Not enough ready peers → timeouts even if hub is fine |
| Control-plane load | Correlate confirms per in-flight write |
| Tail latency | Slow peer dominates unless timeout is tight |

**Guidance:** leave **W = 0** for almost all writes; raise W only where cross-process “others must not serve old VALID” is worth the tax. Measure p99 write vs W.

#### Timeout

| Outcome | Behavior |
|---|---|
| ≥ W confirms in time | OK; optional `confirmed_by` |
| Timeout | Error `WriteConfirmTimeout`; **L2 commit stands**; peers still converge async |

Sync/raised-W is a **visibility wait**, not a multi-node transaction abort.

### 2.2 Who needs a refresh? (partition + interest)

**You do not update all nodes for every write.** Three layers limit work:

```text
                    all L1 processes in the cluster
                              │
              only subscribers of partition P     ◄── stream delivery
                              │
              only L1s that hold key K (or are fetching K)  ◄── SM apply / refetch
```

| Layer | Scope | Behavior |
|---|---|---|
| **L2 ownership** | Key → partition `P` | Write lands only on P’s leader; stream event only on P’s stream. |
| **Stream subscription** | L1 → set of partitions | L1 subscribes to partitions for keys it **holds**, is **fetching**, or is **configured** to pin (e.g. shard-affine pods). It does **not** need every partition by default. |
| **Key apply** | Resident keys only | Invalidation for key K: if slot is `EMPTY` and K is not in flight → advance stream watermark only, **do not** hydrate or refetch. If `VALID`/`STALE`/`FETCHING` → raise ceiling / mark STALE / race rules. |

**Interest rules (normative):**

1. **On first resident miss/fetch of a key in partition P:** ensure subscription to P (or join a process-level subscription already covering P) before or with the Get.
2. **On eviction of the last key in P** (optional optimization): may drop subscription to P after watermark is clean; must re-subscribe before serving K in P again. Simpler v1: keep subscription until process exit if the working set is wide.
3. **Shard-affine deployments:** app pods that only ever touch a key subset pin `subscribed_partitions` / `required_partitions` to that subset—writes in other partitions never touch those L1s.
4. **Peer fanout** (if used) is also interest-aware: forward events only toward peers that advertise interest in that `partition_id`, or skip fanout and use **L2 push to subscribers only** (preferred at scale).
5. **Readiness** requires freshness only for **`required_partitions`** (configured and/or currently subscribed set)—not for the entire cluster keyspace.

**What “refresh” means for a peer that *is* interested:**

- Not “reload the whole cache.”
- Apply invalidation → usually **`STALE` for that one key** → **Get from L2 on next read** (or background revalidate). Other local keys unchanged.

```text
Write key K ∈ partition 3
  → L2-P3 commit + stream_seq
  → only L1s subscribed to P3 receive event
       ├─ hold K?  → STALE K, refetch later
       └─ no K?    → watermark only, no Get
  → L1s not subscribed to P3: untouched
```

**Why not update all?** Bandwidth, CPU, and false sharing: most app processes never held K; forcing them to process every mutation does not improve correctness and destroys scale. Correctness only requires: **every process that might serve a cached K learns of hub commits for K** (via subscription while K is resident or pinned).

## 3. L1 Key State Machine

Each resident key slot has one mutex or an equivalent atomic transition mechanism. The payload and its metadata are published as one immutable record.

### 3.1 States and payload visibility

| State | Payload retained? | Queryable? | Meaning |
|---|---|---|---|
| `EMPTY` | no | no | No resident record; only stream watermarks may exist cluster-wide. |
| `FETCHING` | optional prior | no (unless policy serves retained) | Singleflight to L2 in progress; may keep last payload for stale-serve policy. |
| `VALID` | yes | yes | Authoritative local copy at known version tag `(partition_id, sequence)`. |
| `STALE` | yes (policy-dependent) | yes only if serve-stale policy allows | Invalidation ceiling exceeds local version; payload may still be held for bounded stale serve. |

**Stale-serve policy** (config, default `Never`):

| Policy | Behavior |
|---|---|
| `Never` | `STALE` retains ceiling metadata only; payload is not queryable. Reads must fetch. Errors return error. |
| `StaleIfError` | On fetch error/deadline, if a previous payload was retained, return it with a degraded flag; do not mark the slot `VALID`. |
| `ServeStaleWhileRevalidate` | `STALE` keeps last payload queryable; a read may return it immediately and start/join refresh in the background (or on the same call, depending on API). |

Under `Never`, `VALID → STALE` clears the queryable payload (descriptor may keep ceiling only). Under the other policies, the last immutable payload is retained until a successful refresh or eviction.

### 3.2 Transition table

| State | Event | Next state | Required action |
|---|---|---|---|
| `EMPTY` | read | `FETCHING` | Create a singleflight request to L2. |
| `EMPTY` | invalidation | `EMPTY` | Advance stream watermark only; do not hydrate the key. |
| `FETCHING` | L2 returns authoritative version that is current for epoch and ≥ invalidation ceiling | `VALID` | Atomically publish payload, TTL, epoch, and version; wake waiters. |
| `FETCHING` | L2 returns below ceiling, older epoch, or `NOT_CAUGHT_UP` | stay `FETCHING` or retry via `EMPTY`→`FETCHING` | Discard; bounded exponential backoff within caller deadline. |
| `FETCHING` | invalidation | `FETCHING` | Raise `max_invalidated_version`; do not cancel shared waiters. |
| `FETCHING` | error/deadline | `EMPTY` or `STALE` | Wake waiters with error; if `StaleIfError` and retained payload exists, return degraded stale value without claiming `VALID`. |
| `VALID` | read before TTL | `VALID` | Return the immutable local payload. |
| `VALID` | newer invalidation | `STALE` | Raise ceiling; clear or retain payload per stale-serve policy. |
| `VALID` | equal/older invalidation | `VALID` | Ignore idempotently. |
| `VALID` | TTL reached | `EMPTY` | Evict the record. |
| `VALID` / `STALE` / `EMPTY` / `FETCHING` | local write succeeds (L2 Set ack) | `VALID` | **Read-your-writes (mandatory):** install payload + version from the Set response before returning OK to the app; see §5.1. |
| `VALID` / `STALE` / `EMPTY` / `FETCHING` | local delete succeeds (L2 Delete ack) | `EMPTY` or tombstone `VALID` | Install versioned tombstone / clear queryable payload per delete semantics before returning OK. |
| `STALE` | read | `FETCHING` (and optionally serve retained) | Start or join singleflight; may return retained payload under serve-stale policy. |
| `STALE` | invalidation | `STALE` | Raise the invalidation ceiling. |
| any non-fetching state | memory eviction | `EMPTY` | Remove payload and descriptor. |

**Read-your-writes on the writing L1 is not optional.** Waiting for the invalidation stream (or a later Get) after a successful Set would allow `Get` on the same process to return the pre-write value until fanout catches up. The Set/Delete RPC response carries **version tag** `(partition_id, sequence)` (and value for Set); the writing L1 must apply that to its key slot **before** completing the client call (for W=0; for W>0 the RPC stays open until hub has confirms—see SEMANTICS §8).

### 3.3 Version tag, hub generation, and order

**Authoritative format (SEMANTICS):**

```text
VersionTag = (partition_id, sequence)
```

There is **no** `partition_term` field. Older drafts that included it are void.

| Field | Role |
|---|---|
| **partition_id** | Hub-internal shard; stream identity; fail-closed misroute checks |
| **sequence** | Strictly monotonic commit order per partition under current **hub_generation** |

| Alongside (not in tag) | Role |
|---|---|
| **hub_generation** | Logical hub config/recovery generation; connect + readiness; bump only when sequences cannot continue safely |
| **stream_sequence** | Control-plane delivery order on that partition’s stream |
| **subscriber / node id** | Delivery routing only |

**Ordering** (same `partition_id` only):

```text
(p, s1) < (p, s2)  iff  s1 < s2
p1 != p2 → not comparable as same lineage
```

**No sequence reset** under the same `hub_generation`. Hub HA must preserve per-partition sequence continuity via replicated log. Unsafe restore → new `hub_generation` + L1 revalidation, not a per-key term inside VersionTag.

L1 state:

```text
installed_hub_generation: uint64
partition_state[partition_id]:
  applied_stream_watermark: uint64
  last_origin_checkpoint_at: time
  origin_head_sequence: uint64

// per key slot (only if slot exists — see ApplyInvalidation):
version: VersionTag  // (partition_id, sequence)
max_invalidated: VersionTag | unset
```

**Fetch/invalidation race invariant.** For fetch response `R` and ceiling `I` on key K:

```text
publish R as VALID only when:
  R.partition_id == expected_partition(K)
  AND (I unset OR (R.partition_id == I.partition_id AND R.sequence >= I.sequence))
  AND hub_generation matches installed
```

If L1 observes **hub_generation > installed**: install generation, revalidate resident keys, re-subscribe; readiness false until healthy (`HUB_GENERATION_MISMATCH` while installing).

The fetch request includes `minimum_version` whenever an invalidation ceiling is known. L2 must return a record at least that version or a retryable `NOT_CAUGHT_UP`. Equality is safe: both name the same committed L2 version.

Deletes use versioned tombstones. A missing response without a version cannot supersede a previously observed invalidation.

## 4. Data and Control Protocols

### 4.1 L2 data envelope

All integers use network byte order. The binary envelope is:

| Field | Type | Bytes | Notes |
|---|---:|---:|---|
| magic | `uint8` | 1 | `0xC7` |
| format version | `uint8` | 1 | Envelope schema version, initially `1` |
| flags | `uint16` | 2 | Bit0 tombstone; bit1 compressed (payload is compressed; length is compressed size); bits 2–15 reserved |
| partition id | `uint32` | 4 | Owning partition; **part of version tag** |
| record sequence | `uint64` | 8 | Monotonic per partition; full tag = `(partition_id, sequence)` |
| TTL expiry | `int64` | 8 | Unix nanoseconds; `0` means no expiry |
| payload length | `uint32` | 4 | Bounded by configured maximum; 0 if tombstone |
| checksum | `uint32` | 4 | **Always present.** CRC32C over all prior header fields and the payload bytes |
| payload | bytes | variable | Opaque application bytes (compressed if flag set) |

Checksum is mandatory in format version 1; there is no optional checksum layout. Bounds and checksum are validated before allocating the payload buffer. CRC covers the compressed bytes when compression is enabled. Protobuf or Connect/gRPC may frame RPC requests; the stored value remains opaque.

### 4.2 Identity and sequence spaces

| Space | Assigned by | Scope | Purpose |
|---|---|---|---|
| **hub_generation** | Logical hub config/recovery | Whole hub | Routing map, membership, sequence-space continuity |
| **Version tag** `(partition_id, sequence)` | Hub on commit | Per key commit | Authoritative data ordering (SEMANTICS) |
| **Stream sequence** `stream_sequence` | Logical hub on invalidation publish | Per partition stream | Contiguous delivery / gap detection |

**Invalidation stream identity (v1):**

```text
stream_id = (partition_id)   // under current hub_generation
```

- The **logical hub** assigns `stream_sequence` for each partition (internal sharding only).
- Stream sequences are contiguous on the success path and continue across hub HA if the log is preserved.
- L1 nodes **never** mint or renumber `stream_sequence`.

### 4.3 Acknowledgement layers (normative)

There are **two independent acknowledgement layers**:

| Layer | Message / mechanism | Advances when | Must not mean |
|---|---|---|---|
| **Hop receipt** | TCP-level or `HopFrameAck` between two peers | Bytes received into the peer’s network buffer / decode path | Event applied to any key state machine |
| **Application applied** | `StreamAcknowledgement` | Event applied to L1 key state machine for that origin `stream_id`, **or** (origin/relay only) event durably persisted to a recovery-safe replay log | “Held only in RAM” |

**Rules:**

1. Upstream origin (L2) may reclaim a stream range from its replay log only after every required subscriber has advanced an **application** watermark past that range, or after the subscriber is marked dead and reconciliation is required on rejoin—not after hop receipt alone.
2. An L1 **must not** advance `StreamAcknowledgement.contiguous_stream_sequence` for a sequence that is only queued in memory.
3. Default L1 path: apply to the state machine first, then advance the application watermark, then send `StreamAcknowledgement`.
4. If a node retains events for downstream fanout without applying them (pure relay): it must **durably persist** those events and recover the log before `/readyz` succeeds; only then may it advance a **relay durable** watermark used for origin reclaim. Relays still forward hop-acks separately.
5. Crash after hop-ack but before apply must not allow silent loss: either the application watermark never advanced (origin still has the range), or durable relay recovery reloads the range before ready.

### 4.4 Invalidation and control messages

```protobuf
syntax = "proto3";
package gossipcache.control.v1;

enum Operation {
  OPERATION_UNSPECIFIED = 0;
  OPERATION_INVALIDATE = 1;
  OPERATION_DELETE = 2;
}

// Full version tag — matches SEMANTICS (no partition_term).
message Version {
  uint32 partition_id = 1;
  uint64 sequence = 2;
}

message StreamId {
  uint32 partition_id = 1;
}

message InvalidationEvent {
  Operation operation = 1;
  bytes key = 2;
  Version version = 3;              // full tag (partition_id, sequence)
  StreamId stream_id = 4;
  uint64 stream_sequence = 5;       // contiguous per stream_id; hub-assigned only
  fixed64 origin_hub_id = 6;        // hub identity at publish time
}

// Peer confirmation for hub-aggregated W (SEMANTICS §8).
message InvalidateConfirm {
  fixed64 node_id = 1;
  bytes key = 2;
  Version version = 3;
  uint64 stream_sequence = 4;
}

message InvalidationBatch {
  fixed64 hop_sender_node_id = 1;   // immediate TCP peer (relay or origin)
  StreamId stream_id = 2;
  uint64 first_stream_sequence = 3;
  repeated InvalidationEvent events = 4;
}

// Hop-level only: peer received and decoded the frame. Not application apply.
message HopFrameAck {
  fixed64 receiver_node_id = 1;
  uint64 frame_id = 2;
}

// Application-level ack for an origin stream (never hop receipt alone).
message StreamAcknowledgement {
  fixed64 receiver_node_id = 1;
  StreamId stream_id = 2;
  // Highest contiguous origin sequence applied to the key state machine
  // (L1) or durably persisted for recovery-safe relay (relay role only).
  uint64 contiguous_stream_sequence = 3;
  repeated SequenceRange missing_ranges = 4;
}

message SequenceRange {
  uint64 first = 1;
  uint64 last = 2;
}

// Origin liveness / head advertisement. Required even when the stream is idle.
message StreamCheckpoint {
  StreamId stream_id = 1;
  uint64 head_stream_sequence = 2;   // highest sequence published (may equal last event)
  uint64 hub_generation = 3;
  fixed64 origin_publisher_id = 4;
  int64 wall_time_unix_nanos = 5;    // origin clock; for diagnostics only
}
```

Keys are bytes so the wire contract does not impose UTF-8. Batches have byte and event-count limits. Unknown operations and incompatible schema versions fail closed and increment protocol-error metrics.

## 5. Reliable Invalidation over TCP

### 5.1 Publish path (normative for v1)

This path implements §2.0: **L2 write always updates version propagation**.

1. Client L1 issues Set/Delete/CAS RPC to the owning L2 partition (any other mutator uses the same commit barrier).
2. **L2 single durable commit** (all or nothing):
   - store new value or tombstone;
   - assign version tag `(partition_id, sequence++)`;
   - append changefeed/invalidation with `stream_sequence++` for `stream_id = (partition_id)`.
3. L2 acks the client only after that barrier succeeds. The RPC response includes the full version tag (and for Set, enough for L1 to install the written value). **A committed write without a durable stream event is a protocol bug.**
4. **L2 version propagation (mandatory after commit):** enqueue/push the invalidation onto the origin stream (subscribers + durable replay log). Advance stream head used by `StreamCheckpoint`. Crash between ack and push is recovered by replaying the durable changefeed—not by dropping propagation.
5. **Writing L1 — read-your-writes (mandatory, before returning OK to the app):**
   - **Set:** install payload as `VALID` at the returned version (refresh in-flight `FETCHING` waiters to the new value).
   - **Delete:** install versioned tombstone / clear payload per delete semantics.
   - Align invalidation ceiling so this version is not rejected.
   - Do **not** wait for the invalidation stream for local install; the stream is how **other** processes learn.
6. **Return policy (tunable W, §2.3):** if **W = 0** (default), return OK now; peer propagation continues async. If **W > 0**, block until W peer confirms at `ConfirmLevel` or timeout—**no L2 rollback** on timeout.
7. Peer L1s apply hub events → typically `STALE` → refetch on read; application watermark + confirm/ack (§4.3).
8. Writer receiving its own invalidation for the **same** version: idempotent (stay `VALID`).
9. Gaps → replay; expired → `RECONCILIATION_REQUIRED` + anti-entropy before ready.

Writing L1s **must not** mint stream sequences or publish as origin. L2 **must** advance version tag + stream sequence on every durable mutation. **W only tunes how many peer confirms gate the client OK**; it does not change the hub commit.

### 5.2 Delivery algorithm (partition subscribers, not all nodes)

**Default at scale:** L2 is the fanout origin **per partition**. Each L1 maintains subscriptions only for partitions in its interest set (§2.2). This is **not** “send every invalidation to every process.”

Frames are length-prefixed and multiplex invalidation batches, hop acks, stream acknowledgements, stream checkpoints, replay requests, and anti-entropy messages over mTLS TCP.

1. Origin (L2 partition leader) assigns `stream_sequence` and retains events in a durable bounded replay log **for that partition stream**.
2. L2 pushes batches to **current subscribers of that stream** only. Optional L1↔L1 fanout is secondary and should be **interest-filtered** by `partition_id` (or omitted in v1 in favor of direct L2→subscriber push).
3. Relays (if any) forward idempotently (dedupe by `(stream_id, stream_sequence)` or key+version). `HopFrameAck` = receipt only. Origin reclaim uses application / durable-relay watermarks (§4.3).
4. On receive: advance stream watermark; **apply key SM only if the key is resident or in `FETCHING`** (§2.2). Non-holders do not refetch.
5. Gaps → replay from L2; overflow → reconciliation-required, never silent drop while ready.
6. On reconnect: exchange watermarks for **subscribed** streams only; re-subscribe before serving keys in a partition.

Backpressure is mandatory per subscription.

Transport security: TLS 1.3 with workload identity/mTLS. Connection establishment validates cluster ID and `hub_generation`.

### 5.3 Stream freshness and subscription leases

Freshness is tracked **per subscribed / required partition**, not cluster-wide.

A subscriber that **stops receiving the tail** of a stream it still needs may see no gap and would otherwise stay ready forever while serving stale **resident** keys for that partition. Gap detection alone is insufficient.

**Origin obligations:**

1. For every active partition stream, L2 periodically emits `StreamCheckpoint` even when idle.
2. Each checkpoint advertises `head_stream_sequence` and `hub_generation`.
3. **Subscription lease** per stream: subscribers refresh within `lease_ttl` or are dropped and must re-subscribe before relying on that partition’s cache.

**L1 obligations:**

1. For every **required** partition (pinned config ∪ currently subscribed interest set), track `last_origin_checkpoint_at` and `origin_head_sequence`.
2. If `now - last_origin_checkpoint_at > stream_freshness_timeout` for a required partition → **`STREAM_FRESHNESS_UNKNOWN`** (unready for traffic that depends on that cache, or process-wide unready if the process serves those keys).
3. Partitions the process never subscribed to do **not** affect readiness.
4. Checkpoint head ahead of applied watermark with holes → gap/replay path.
5. Optional peer loss ≠ unready; origin freshness for **required** partitions does.

**Config knobs:**

| Knob | Meaning |
|---|---|
| `stream_checkpoint_interval` | Origin emit period for `StreamCheckpoint` |
| `stream_freshness_timeout` | Max age of last origin checkpoint before unready |
| `subscription_lease_ttl` | Origin-side lease for subscribers |
| `required_partitions` | Pinned partitions that must be fresh for `/readyz` |
| `subscribe_on_demand` | Auto-subscribe partition on first key interest (default true) |
| `unsubscribe_when_empty` | Drop subscription when no resident keys in partition (optional) |

Metrics: `gossipcache_stream_checkpoint_age_seconds`, `gossipcache_stream_freshness_timeouts_total`, `gossipcache_stream_subscribers` (bounded labels: partition id ok if cardinality capped).

## 6. Anti-Entropy and Read Safety

Periodic anti-entropy is required even with TCP because nodes can be offline longer than replay retention.

- L2 exposes **per-partition** version summaries (Merkle tree or range digest) and a durable changefeed cursor.
- L1 anti-entropy and invalidation apply only for **keys it currently holds** (and partitions it subscribes to); it does not warm absent keys or reconcile the whole cluster keyspace.
- A mismatch moves the local slot to `STALE` using the authoritative version ceiling.
- Readiness is false after a detected unrecoverable sequence gap until reconciliation completes.
- Readiness is also false when origin stream freshness cannot be validated (§5.3), even if no gap was observed.
- Optional maximum-staleness policy forces L1 to revalidate with L2 before serving a hit older than the configured interval. Without this policy, backed mode remains eventually consistent within the stale-serve configuration.

## 7. L2 Hub Plan

### 7.1 Correctness-first baseline

- Partitioned ownership with one event loop per shard.
- Durable write-ahead journal for epoch, versions, values, TTLs, and tombstones.
- In-memory hash index and hot payload tier.
- Append-only segment files for warm/cold payloads.
- Checksummed recovery and compaction.
- Snapshot plus journal replay.
- Replication and leader/fencing rules per partition.
- Changefeed cursor advanced atomically with the mutation commit (§2.0): no committed write without version + stream event.

### 7.2 Optimization gates

Cache-line-aware hashing, `io_uring`, `O_DIRECT`, mmap offsets, fibers, and thread-per-core execution are optimizations, not correctness dependencies. Add each behind an interface only after a benchmark identifies the bottleneck. `io_uring` and `O_DIRECT` are Linux-specific; buffered I/O remains the development and portability path.

Hot-tier admission uses a configurable byte budget (TinyLFU/LFU), not a fixed “hottest 15%” rule. Index overhead is included in capacity accounting.

## 8. Failure Behavior

| Failure | Behavior | Recovery |
|---|---|---|
| L1 pod starts/scales | Slots begin `EMPTY`; readiness does not wait for cache warming but requires stream freshness on required partitions. | Demand-fill from L2; subscribe; install cluster generation + partition terms. |
| MicroVM boots/restores (optional profile) | Guest and cache readiness remain false until identity, clock, network, volume, recovery, generations, and watermarks are validated. | Cold boot or snapshot-specific reconciliation; never reuse snapshotted ready state. |
| TCP connection breaks | Local hits may continue under staleness policy; application watermarks only reflect applied work. | Reconnect; if checkpoints stop, `STREAM_FRESHNESS_UNKNOWN`; replay or anti-entropy. |
| Silent stream stall (no gap, no checkpoints) | Not ready (`STREAM_FRESHNESS_UNKNOWN`); may still serve per stale policy. | Restore origin checkpoints / re-subscribe. |
| Replay gap exceeds retention | `RECONCILIATION_REQUIRED`. | Held-key summaries vs L2 before readiness. |
| L2 RPC unavailable | Misses fail or use stale-serve policy; writes fail closed. | Retry with jitter and circuit breaking. |
| L2 partition leader fails | Fencing prevents dual writers in the same **partition term**. | Promote replica: continue term under fence, or advance **that partition’s term only**. |
| Network partition | Bounded-stale L1 hits per policy; only fenced owner accepts writes. | Replay + anti-entropy + freshness restore after heal. |
| Disk corruption | Do not serve unchecked records. | Restore replica/snapshot and replay journal. |

## 9. Observability Contract

Observability is part of correctness: a node must expose whether it is safe to serve, not merely whether its process is alive. Metrics use OpenTelemetry-compatible instruments with a Prometheus endpoint.

### 9.1 Ownership split

| Layer | Owns |
|---|---|
| **H1–H3** | No-op meter/tracer interfaces; emit hooks on state transitions, RPC, and stream events |
| **H4** | Minimum production safety signals: readiness reason codes, reconciliation flag, gap/replay counters, L2 durability errors; K8s probe wiring |
| **Phase 5** | Full metric catalog validation, dashboards, SLO burn-rate alerts, runbooks, cardinality CI, overhead budgets, fault-to-signal matrix |

H4 must not claim dashboards/SLO burn-rate delivery as done; Phase 5 owns that exit gate.

### 9.2 Metrics

Use counters and histograms rather than client-side quantiles. Deployment metadata (pod, node, zone, cluster) comes from the collector or scrape target labels—not application labels.

| Area | Required instruments |
|---|---|
| L1 reads | `gossipcache_l1_requests_total{result=hit\|miss\|stale_hit\|error}`, `gossipcache_l1_get_duration_seconds{result}`, `gossipcache_l1_resident_bytes`, `gossipcache_l1_entries{state}`, `gossipcache_l1_evictions_total{reason}` |
| State machine | `gossipcache_l1_transitions_total{from,to,reason}`, `gossipcache_l1_fetches_inflight`, `gossipcache_l1_fetch_retries_total{reason}`, `gossipcache_l1_singleflight_waiters`, `gossipcache_l1_stale_response_rejected_total` |
| Invalidation | `gossipcache_invalidation_events_total{direction,result}`, `gossipcache_invalidation_apply_duration_seconds`, `gossipcache_invalidation_duplicate_total`, `gossipcache_invalidation_queue_depth`, `gossipcache_invalidation_queue_capacity`, `gossipcache_invalidation_coalesced_total` |
| Reliability | `gossipcache_peer_connections{state}`, `gossipcache_peer_reconnects_total{reason}`, `gossipcache_replay_events_total{result}`, `gossipcache_replay_lag_events`, `gossipcache_sequence_gaps_total`, `gossipcache_sequence_gap_age_seconds`, `gossipcache_ack_latency_seconds`, `gossipcache_reconciliation_required`, `gossipcache_stream_checkpoint_age_seconds`, `gossipcache_stream_freshness_timeouts_total` |
| Anti-entropy | `gossipcache_antientropy_runs_total{trigger,result}`, `gossipcache_antientropy_duration_seconds`, `gossipcache_antientropy_keys_compared_total`, `gossipcache_antientropy_mismatches_total` |
| L2 RPC | `gossipcache_l2_rpc_requests_total{method,code}`, `gossipcache_l2_rpc_duration_seconds{method}`, `gossipcache_l2_rpc_inflight{method}`, `gossipcache_l2_not_caught_up_total` |
| L2 storage | `gossipcache_l2_operations_total{operation,result}`, `gossipcache_l2_operation_duration_seconds{operation,tier}`, `gossipcache_l2_tier_bytes{tier}`, `gossipcache_l2_tier_access_total{tier}`, `gossipcache_l2_journal_lag_bytes`, `gossipcache_l2_fsync_duration_seconds`, `gossipcache_l2_compaction_duration_seconds`, `gossipcache_l2_disk_free_bytes` |
| Version safety | `gossipcache_hub_generation`, `gossipcache_version_sequence`, `gossipcache_version_regression_total` |

Label values must be bounded enums. Never label with key, peer address, request ID, sequence, error text, tenant ID, or raw node ID.

### 9.3 Tracing

- Trace misses, refreshes, writes, L2 RPCs, replay sessions, and anti-entropy—not every L1 hit.
- Propagate W3C trace context over L2 RPC. Fanout uses linked spans, not one unbounded cluster span.
- Always sample errors, stale-response rejection, gaps, reconciliation, version regressions, and slow requests.

### 9.4 Structured logs and audit events

Stable event names include: `sequence_gap_detected`, `replay_started`, `replay_unavailable`, `reconciliation_started`, `reconciliation_completed`, `stale_fetch_rejected`, `peer_backpressured`, `hub_generation_changed`, `version_regression_detected`, `hub_generation_mismatch`, `stream_freshness_unknown`, `disk_corruption_detected`.

Do not log values, credentials, TLS material, or raw keys.

### 9.5 Health and readiness

| Endpoint | Meaning |
|---|---|
| `/livez` | Event loops advance; process not deadlocked. Does not test L2 or peers. |
| `/readyz` | Protocol compatible; required L2 routes available; no unreconciled stream gap; **origin stream freshness OK for every required partition**; replay/queue pressure below threshold; recovery complete; cluster generation and required partition terms installed; durable relay logs recovered if acting as relay. |
| `/startupz` | Local recovery and listeners initialized. |
| `/debug/cache` | Authenticated aggregate state counts. |
| `/debug/peers` | Authenticated peer/stream watermarks, checkpoint age, replay lag, queue pressure. |

Reason codes include `RECONCILIATION_REQUIRED`, `L2_ROUTE_UNAVAILABLE`, `REPLAY_OVERFLOW`, `PROTOCOL_INCOMPATIBLE`, `HUB_GENERATION_MISMATCH`, **`STREAM_FRESHNESS_UNKNOWN`**.

Optional fanout peer loss alone does not fail readiness. **Missing origin checkpoints** for a required partition does.

**Primary production profile: Kubernetes.** Startup/liveness/readiness probes and graceful drain are required for the first release.

**Optional profile: MicroVM.** Guest boot, clock, volume, incarnation ID, vsock status mirroring, and snapshot restore rules are specified in [Phase 5](PHASE_5_OBSERVABILITY.md#optional-microvm-readiness-profile) and are not a Phase 5 exit gate for the first release.

### 9.6 Dashboards, SLOs, and alerts

Four baseline dashboards (Phase 5): L1 effectiveness, invalidation/replay, L2 RPC/storage, durability/capacity.

Page on version regression, fencing failure, corruption, acknowledged-write loss, unready replicas below quorum, unreconciled gap past staleness budget. Warn on replay lag, queue pressure, reconnect storms, mismatch rate, L2 tail latency, compaction backlog, disk forecast.

### 9.7 Acceptance tests

See Phase 5. Telemetry failure must never block L1 reads, invalidation application, or L2 commits.

## 10. Implementation Sequence

### Milestone H1: Contracts and state machine

- Finalize envelope and protobuf schemas, compatibility policy, golden fixtures.
- Implement L1 state machine, immutable publication, TTL, singleflight, tombstones, stale-serve policies, epoch rules, race tests.
- Benchmarks for hit latency, concurrent misses, allocations, invalidation apply.

**Exit gate**: Every transition covered; stale fetch cannot become `VALID`; benchmarks name hardware and load profile.

### Milestone H2: Reliable control plane

- Framed mTLS TCP, fanout, batching, hop acks vs application `StreamAcknowledgement`, durable origin replay log, gap detection, coalescing, backpressure.
- `StreamCheckpoint` heartbeats, subscription leases, freshness timeout → `STREAM_FRESHNESS_UNKNOWN`.
- L2 changefeed subscription as origin; L1 does not mint sequences.
- Fault tests: disconnect, truncation, slow peer, duplicate, reconnect reorder, cert rotation, **silent stall without gap**, **crash after hop-ack before apply**.

**Exit gate**: No silent loss inside replay window; application acks never advance on RAM-only buffers; expired gaps force reconciliation; silent stall fails readiness with `STREAM_FRESHNESS_UNKNOWN`.

### Milestone H3: Authoritative L2 baseline

- Shard ownership, Get/Set/Delete/CAS, durable journal, RAM index, disk segments, recovery, compaction, changefeed.
- Mutation and changefeed cursor durable atomically.
- Dev single-node OK; production path requires replica + fencing design implemented enough for failover tests.

**Exit gate**: Crash/restart and failover never regress versions or ack a lost write.

### Milestone H4: Anti-entropy, minimum observability, Kubernetes ops

- Held-key summaries, immediate reconciliation on gaps, periodic sweeps.
- Kubernetes discovery, PDBs, topology spread, rolling-upgrade compatibility.
- **Minimum** metrics/hooks and full readiness reason codes (not full dashboard/alert suite).

**Exit gate**: Chaos covers pod churn, partitions, slow peers, L2 failover, offline beyond replay retention; readiness never lies about unreconciled gaps.

### Milestone H5: Hardware optimization

- Only after baseline profiling: thread-per-core, io_uring, direct I/O, adaptive hot tier, etc.
- Portable engine remains the reference.

**Exit gate**: Each optimization has benchmark, metric, rollback switch, no correctness regression.

### Phase 5 (after H4 interfaces stable)

End-to-end observability delivery: dashboards, SLO alerts, cardinality CI, overhead budgets, fault-to-signal matrix. See [PHASE_5_OBSERVABILITY.md](PHASE_5_OBSERVABILITY.md).

## 11. Required Test Matrix

- Every state/event pair, including all stale-serve policies.
- Invalidation before/during/after fetch; duplicate/older/newer/cross-epoch.
- Partition term advance mid-fetch and mid-fanout (other partitions unaffected).
- Cluster generation change vs single-partition failover.
- Application ack only after apply; crash between hop-ack and apply.
- Silent stream stall (checkpoints stop) while ready would otherwise stay true.
- TTL vs read/fetch/eviction/invalidation races.
- Singleflight same-key and unrelated-key parallelism.
- TCP partial frame and ack disconnect.
- Replay available, truncated, queue overflow, slow peer.
- L2 crash before/after commit+changefeed ack.
- Leader failover continuing stream vs epoch advance.
- Tombstone retention; no deleted-key resurrection.
- Adjacent protocol version rolling upgrade.
- Telemetry collector outage and export backpressure.
- Cardinality, readiness-reason, fault-to-alert coverage (Phase 5).

## 12. Explicit Non-Goals for the First Release

- Custom RUDP transport.
- Cross-region synchronous consistency.
- Full-value peer gossip in backed mode.
- Automatic L1 pre-warming.
- Hardware-specific L2 optimizations before baseline profiling.
- Claiming sub-microsecond latency without reproducible percentile benchmarks.
- Redis/Postgres as the backed-mode source of truth.
- L1-originated invalidation publish (dual publisher).
- MicroVM as a required production profile.

## 13. Performance and Load Profile (release gates)

Gates are measured, not aspirational. Until baseline runs exist, numeric SLOs stay provisional.

**Reference load profile (document results against this or a recorded successor):**

| Parameter | Reference value |
|---|---|
| L1 processes | 10 |
| L2 partitions / leaders | 3 partitions, 1 leader + 1 follower each |
| Working set | 1e6 keys, value size 1 KiB (p50), 16 KiB (p99) |
| Read:write ratio | 95:5 |
| Gossip fanout | 3 |
| Invalidation batch max | 64 events or 32 KiB |
| Injected faults | 1 peer restart per 10 min; 100ms delay on one link |

**Provisional gates (revisit after measurement):**

| Gate | Provisional target |
|---|---|
| L1 hit latency | p99 &lt; 1 ms on named hardware (no alloc on hit path) |
| Invalidation apply convergence (origin publish → all ready L1s) | p99 &lt; 500 ms under the reference profile with no partitions |
| Unreconciled gap after replay expiry | readiness false within 1 s of detection |
| Acknowledged write durability | zero acknowledged loss under crash tests |

## 14. Related docs

| Document | Role |
|---|---|
| [../SEMANTICS.md](../SEMANTICS.md) | **Wins on conflict** — locked product semantics |
| This file | Implementation detail, wire, milestones H1–H5 |
| [PHASE_5_OBSERVABILITY.md](PHASE_5_OBSERVABILITY.md) | Observability delivery phase |
| [../ARCHITECTURE.md](../ARCHITECTURE.md) | Short overview |
| [../TECHNICAL_SPEC.md](../TECHNICAL_SPEC.md) | API/types sketch |
| [../diagrams/SEQUENCES.md](../diagrams/SEQUENCES.md) | Flows |

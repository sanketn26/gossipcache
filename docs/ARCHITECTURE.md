# GossipCache Architecture

**Caches must be local.** Hot reads stay in-process. Consistency is hub-mediated.

**Locked semantics and choices:** [SEMANTICS.md](SEMANTICS.md)
**Implementation phases:** [impl/README.md](impl/README.md)

## Shape (v1)

```text
  App + L1 ──RPC miss/write──►  L2 hub (memory-first runtime authority)
     ▲                            │
     └──── invalidation stream ───┘  (key + version only; then fetch on read)
```

| Piece | Role |
|-------|------|
| **Node (L1)** | Embedded library: hits, state machine, singleflight, hub RPC and stream consumer |
| **Hub (L2)** | Separate memory-first service: runtime values and versions; sole invalidation publisher; durability opt-in |
| **Partitions** | Internal hub scale (journals/streams)—not “one app node per partition” |

Independent full-value gossip is **out of scope for v1**.

## Data vs control

| | Carries | Path |
|--|---------|------|
| Data | Values | L1 hit, or L2 RPC |
| Control | Invalidations | Hub → interested L1s (mTLS TCP) |

## Paths (summary)

**Read:** local `VALID` hit → else L2 Get → install if version ≥ invalidation ceiling.
**Write:** L2 profile barrier (value + version + stream event in memory by default;
synchronously persisted when opted in) → writer installs locally → OK per **W**
(default 0) → peers invalidate then fetch.

## Hub storage profiles

| Profile | Behavior | Principal bottlenecks |
|---------|----------|-----------------------|
| `memory` (default) | All Hub reads/writes in memory; restart starts empty with a new generation | Dataset must fit RAM; restart loses cache contents and forces Node revalidation |
| `durable` (opt-in) | Same memory read path; supports Fast asynchronous persistence and explicit Sync fencing | Sync tail latency, Fast backlog/queueing, write amplification/compaction, disk capacity, recovery time and operational upgrades |

Durability preserves restart state but does not by itself provide replication or
machine/volume-failure HA.

Writes select an acknowledgement mode independently of `W`:

| Write mode | Acknowledgement |
|------------|-----------------|
| `WriteFast` (default) | After atomic Hub memory commit; durable-capable Hubs persist through an ordered asynchronous queue |
| `WriteSync` | After this mutation and all earlier mutations in its partition are synchronously persisted; unavailable on a memory-only Hub |

`WriteSync` can be bottlenecked by the earlier Fast-write backlog. `W` separately
controls how many Nodes confirm invalidation application.
**W:** tunable peer-confirm wait; default async; higher W optional and costly. See [SEMANTICS §8](SEMANTICS.md#8-visibility-and-tunable-w).

## Ops sketch

- **K8s primary:** app Deployment (L1 linked in), L2 StatefulSet + PVC, consistency-aware `/readyz`
- **Ports (placeholders):** 7400 L2 RPC, 7401 streams, 8081 probes, 9090 metrics
- **Detail:** [DEPLOYMENT.md](DEPLOYMENT.md)

## Observability

Readiness is a consistency signal (gaps, stream freshness). See [SEMANTICS §10](SEMANTICS.md#10-readiness) and the [P5 phase files](impl/README.md).

## See also

| Doc | Use |
|-----|-----|
| [SEMANTICS.md](SEMANTICS.md) | Full semantics + decisions |
| [diagrams/SEQUENCES.md](diagrams/SEQUENCES.md) | Read/write/stream flows |
| [TECHNICAL_SPEC.md](TECHNICAL_SPEC.md) | Types and API sketch |
| [impl/README.md](impl/README.md) | Common, Hub and Node phase plans |

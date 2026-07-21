# GossipCache Architecture

**Caches must be local.** Hot reads stay in-process. Consistency is hub-mediated.

**Locked semantics and choices:** [SEMANTICS.md](SEMANTICS.md)
**Implementation plan:** [impl/HYBRID_BACKED_MODE.md](impl/HYBRID_BACKED_MODE.md)

## Shape (v1)

```text
  App + L1 ──RPC miss/write──►  L2 hub (authoritative)
     ▲                            │
     └──── invalidation stream ───┘  (key + version only; then fetch on read)
```

| Piece | Role |
|-------|------|
| **L1** | Embedded library: hits, state machine, singleflight, stream consumer |
| **L2 hub** | Source of truth; durable versions; sole invalidation publisher |
| **Partitions** | Internal hub scale (journals/streams)—not “one app node per partition” |

Independent full-value gossip is **out of scope for v1**.

## Data vs control

| | Carries | Path |
|--|---------|------|
| Data | Values | L1 hit, or L2 RPC |
| Control | Invalidations | Hub → interested L1s (mTLS TCP) |

## Paths (summary)

**Read:** local `VALID` hit → else L2 Get → install if version ≥ invalidation ceiling.
**Write:** L2 durable barrier (value + version + stream event) → writer installs locally → OK per **W** (default 0) → peers invalidate then fetch.
**W:** tunable peer-confirm wait; default async; higher W optional and costly. See [SEMANTICS §8](SEMANTICS.md#8-visibility-and-tunable-w).

## Ops sketch

- **K8s primary:** app Deployment (L1 linked in), L2 StatefulSet + PVC, consistency-aware `/readyz`
- **Ports (placeholders):** 7400 L2 RPC, 7401 streams, 8081 probes, 9090 metrics
- **Detail:** [DEPLOYMENT.md](DEPLOYMENT.md)

## Observability

Readiness is a consistency signal (gaps, stream freshness). Metrics/traces: [SEMANTICS §10](SEMANTICS.md#10-readiness), [impl/PHASE_5_OBSERVABILITY.md](impl/PHASE_5_OBSERVABILITY.md).

## See also

| Doc | Use |
|-----|-----|
| [SEMANTICS.md](SEMANTICS.md) | Full semantics + decisions |
| [diagrams/SEQUENCES.md](diagrams/SEQUENCES.md) | Read/write/stream flows |
| [TECHNICAL_SPEC.md](TECHNICAL_SPEC.md) | Types and API sketch |
| [impl/TESTING_STRATEGY.md](impl/TESTING_STRATEGY.md) | What to test |

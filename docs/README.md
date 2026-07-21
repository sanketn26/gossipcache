# Documentation

## Start here

| Doc | Purpose |
|-----|---------|
| **[SEMANTICS.md](SEMANTICS.md)** | **Locked v1 semantics and design choices** |
| **[STATUS.md](STATUS.md)** | **What is actually implemented** |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Short system shape |
| [diagrams/SEQUENCES.md](diagrams/SEQUENCES.md) | Read/write/stream flows |
| [DEPLOYMENT.md](DEPLOYMENT.md) | K8s / Docker / EC2 sketch |
| [TECHNICAL_SPEC.md](TECHNICAL_SPEC.md) | Thin API/types sketch |

## Implementation

| Doc | Purpose |
|-----|---------|
| [impl/PHASE_PLAN.md](impl/PHASE_PLAN.md) | **Phases P0–P8 + functional deliverables** |
| [impl/HYBRID_BACKED_MODE.md](impl/HYBRID_BACKED_MODE.md) | Wire, SM detail |
| [impl/TESTING_STRATEGY.md](impl/TESTING_STRATEGY.md) | What to test |
| [impl/PHASE_05_OBSERVABILITY.md](impl/PHASE_05_OBSERVABILITY.md) | P5 observability |
| [impl/PHASE_06_SECURITY.md](impl/PHASE_06_SECURITY.md) | P6 security |
| [impl/PHASE_07_DEMO_POLISH.md](impl/PHASE_07_DEMO_POLISH.md) | P7 demo / polish |
| [impl/README.md](impl/README.md) | Impl index + sequence |

## Reading order

1. SEMANTICS
2. ARCHITECTURE + SEQUENCES
3. HYBRID (when implementing)
4. TESTING_STRATEGY
5. DEPLOYMENT when shipping

Older multi-file phase dumps (Redis-era, independent mode, gap analysis) were removed; recover from **Git history** if needed. Prefer SEMANTICS when anything conflicts.

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
| [impl/README.md](impl/README.md) | **Common contract, Hub and Node file for every phase** |
| [impl/README.md#test-strategy-and-phase-gates](impl/README.md#test-strategy-and-phase-gates) | Test pyramid, integration/fault matrix and P0–P8 exit gates |

## Reading order

1. SEMANTICS
2. ARCHITECTURE + SEQUENCES
3. The matching Common, Hub or Node phase file
4. The implementation index test gates
5. DEPLOYMENT when shipping

Older multi-file phase dumps (Redis-era, independent mode, gap analysis) were removed; recover from **Git history** if needed. Prefer SEMANTICS when anything conflicts.

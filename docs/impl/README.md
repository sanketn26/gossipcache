# Implementation roadmap

**Product rules:** [../SEMANTICS.md](../SEMANTICS.md) (authoritative)  
**Phased build (code-level FDs):** [PHASE_PLAN.md](PHASE_PLAN.md)  
**Wire/algorithms:** [HYBRID_BACKED_MODE.md](HYBRID_BACKED_MODE.md)  
**Tests:** [TESTING_STRATEGY.md](TESTING_STRATEGY.md)

## Phase sequence (P0 → P8)

| Phase | Name | Where |
|------:|------|--------|
| P0 | Foundation | [PHASE_PLAN.md](PHASE_PLAN.md) |
| P1 | L1 state machine | PHASE_PLAN |
| P2 | Control plane | PHASE_PLAN |
| P3 | L2 durable hub | PHASE_PLAN |
| P4 | Anti-entropy, health, K8s, min metrics | PHASE_PLAN |
| P5 | Observability suite | [PHASE_05_OBSERVABILITY.md](PHASE_05_OBSERVABILITY.md) |
| P6 | Security hardening | [PHASE_06_SECURITY.md](PHASE_06_SECURITY.md) |
| P7 | Demo, polish, sponsorship | [PHASE_07_DEMO_POLISH.md](PHASE_07_DEMO_POLISH.md) |
| P8 | Performance | PHASE_PLAN |

```text
P0 → P1 → P2 → P3 → P4 → P5 → P8
                    ↘︎     ↘︎
                     P6     P7
```

## Docs in this folder

| File | Role |
|------|------|
| [PHASE_PLAN.md](PHASE_PLAN.md) | **Code-level phased plan + FD checklist (P0–P8)** |
| [HYBRID_BACKED_MODE.md](HYBRID_BACKED_MODE.md) | Wire/SM detail (SEMANTICS wins); H1–H5 map to P1–P4 + P8 |
| [TESTING_STRATEGY.md](TESTING_STRATEGY.md) | Test strategy |
| [PHASE_05_OBSERVABILITY.md](PHASE_05_OBSERVABILITY.md) | P5 detail |
| [PHASE_06_SECURITY.md](PHASE_06_SECURITY.md) | P6 detail |
| [PHASE_07_DEMO_POLISH.md](PHASE_07_DEMO_POLISH.md) | P7 detail |
| [IMPLEMENTATION_GUIDE.md](IMPLEMENTATION_GUIDE.md) | **Legacy** Redis/gossip walkthrough — non-normative; use PHASE_PLAN |

## Decisions

See [SEMANTICS.md §13](../SEMANTICS.md#13-decisions-log-condensed). Prefer SEMANTICS when any impl doc conflicts.

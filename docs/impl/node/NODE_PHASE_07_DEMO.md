# Node P7 — Packaging and demo

**Depends on:** [NODE_PHASE_04_OPERATIONS.md](NODE_PHASE_04_OPERATIONS.md); uses
Node P5/P6 profiles when enabled.

**Common contract:** [COMMON_PHASE_07_DEMO.md](../common/COMMON_PHASE_07_DEMO.md).

## Functional work

- [ ] Provide an example application embedding the node library.
- [ ] Run at least two isolated application processes against one hub.
- [ ] Expose local-hit, miss, write, delete and readiness demonstrations.
- [ ] Script stream interruption, node restart and convergence scenarios.

## Implementation detail

### Example application (`examples/server`)

- Extends the existing local example to embed the node `Client` and point
  `GOSSIPCACHE_L2_ADDRESSES` at the demo hub; built behind `-tags example`.
- Runs as two independent OS processes (or two Compose app containers) so the
  demo shows genuine cross-process convergence, not two clients in one process.
- Exposes an HTTP surface: `GET /cache/{key}` (local hit path), `PUT/DELETE`
  (hub mutation), and `/readyz` proxying node readiness.

### Scenario harness

- `scripts/demo/node_*.sh` drive the two apps and assert via readiness/metrics:
  a write on app A invalidates and refreshes app B; a restarted app demand-fills
  on read without any global warm; stream interruption drops readiness and
  recovers after replay.
- The demo prints `local_result_total{result=hit}` deltas to make the
  "no hub read on a valid hit" property visible.

## Verification

- [ ] Clean checkout demonstrates local hits with no hub read.
- [ ] A write on one node eventually invalidates and refreshes another.
- [ ] Node restart demand-fills without global cache warming.

**Exit:** users can see the distinction between an embedded node and the
separately deployed authoritative hub.

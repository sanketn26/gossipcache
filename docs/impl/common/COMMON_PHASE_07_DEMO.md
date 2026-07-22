# Common P7 — Demo contract

**Depends on:** [COMMON_PHASE_04_OPERATIONS.md](COMMON_PHASE_04_OPERATIONS.md);
uses P5/P6 when observability/security profiles are demonstrated.

- [ ] Freeze topology: one memory Hub by default and at least two
  application-embedded Nodes; durable Hub is an opt-in profile.
- [ ] Script write convergence, update, delete, Node restart, memory Hub restart
  and durable Hub restart.
- [ ] Assert readiness before scenarios and expected transitions during faults.
- [ ] Show local hits separately from Hub miss/write traffic.
- [ ] Compare WriteFast latency with WriteSync fence latency under an observable
  Fast persistence backlog.
- [ ] Avoid claims of production HA in the single-Hub demonstration.

## Implementation detail

### Topology and orchestration

- Default: `docker compose up` brings up one `l2` (memory profile, no volume)
  and two app containers each embedding the node library, pointing
  `GOSSIPCACHE_L2_ADDRESSES` at the hub.
- Opt-in durable profile: `docker compose --profile durable up` adds a named
  volume, `GOSSIPCACHE_HUB_STORAGE_PROFILE=durable`, and a durable-writability
  health check on the hub.
- Scenarios run as a `scripts/demo/*.sh` (or a `cmd/demo` Go harness) that
  drives the public client and asserts on management-port readiness/metrics —
  no sleeps; poll `/readyz` and metric deltas.

### Scripted scenarios and assertions

| Scenario | Assertion source |
|----------|------------------|
| Write convergence | node B local read reflects node A write after invalidation |
| Update / delete | version increases; tombstone read-your-writes on writer |
| Node restart | restarted node demand-fills; no global warm |
| Memory hub restart | `hub_generation` changes; nodes revalidate; cache empty |
| Durable hub restart | Sync-acked keys recovered; streams resume |
| Fast vs Sync | `WriteSync` latency > `WriteFast` under an induced persist backlog |

- Assert readiness **before** each scenario and the expected `ReadyReason`
  transition during each induced fault.
- Instrument `local_hit_total` separately from `rpc_duration` so the demo shows
  hot reads never touch the hub.
- Explicit banner in output: single hub is a development topology, **not**
  production HA.

**Exit:** a clean checkout proves the component boundary and hub-mediated
convergence with repeatable assertions.

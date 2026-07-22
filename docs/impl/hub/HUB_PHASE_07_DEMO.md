# Hub P7 — Packaging and demo

**Depends on:** [HUB_PHASE_04_OPERATIONS.md](HUB_PHASE_04_OPERATIONS.md); uses
Hub P5/P6 profiles when enabled.

**Common contract:** [COMMON_PHASE_07_DEMO.md](../common/COMMON_PHASE_07_DEMO.md).

## Functional work

- [ ] Package `cmd/l2` with explicit data, RPC, control and management config.
- [ ] Run the default demo with a memory Hub and no volume.
- [ ] Add an opt-in durable Compose profile with a volume and health checks.
- [ ] Include restart scenarios showing memory-mode loss/generation change and
  durable-mode recovery.
- [ ] Include Fast versus Sync write scenarios and show that W is a separate
  confirmation control.
- [ ] Document single-hub development limits without implying production HA.

## Implementation detail

### `cmd/l2` packaging

- Single static binary; all config via flags/env (`GOSSIPCACHE_HUB_STORAGE_PROFILE`,
  `..._DATA_DIR`, `..._PARTITION_COUNT`, listener addresses). Memory profile
  needs no volume.
- Multi-stage `deployments/docker/l2.Dockerfile` producing a distroless image;
  memory profile runs with no mounted volume, durable profile mounts one at
  `DataDir`.

### Compose profiles

```yaml
# default: memory hub + 2 app nodes, no volume
# --profile durable: adds volume + GOSSIPCACHE_HUB_STORAGE_PROFILE=durable
#   and a healthcheck asserting durable writability before nodes start
```

### Restart demonstrations

- Memory restart: kill `l2`, restart, show `hub_generation` changed and nodes
  transition through `GenerationRevalidating` to empty-but-ready.
- Durable restart: kill `l2`, restart, show `RecoveryInProgress` → `Ready` with
  Sync-acked keys intact and streams resumed.
- Fast vs Sync: induce a persist backlog (throttled volume) and show `WriteSync`
  latency exceeds `WriteFast`, while `W` is demonstrated as a separate
  peer-confirm control. Output banners the single-hub / no-HA caveat.

## Verification

- [ ] Clean checkout starts one hub and at least two nodes.
- [ ] Memory restart starts empty and invalidates old-generation Node state.
- [ ] Durable restart preserves acknowledged data and resumes streams.

**Exit:** the demo visibly proves memory-first behavior, invalidation origin and
the explicit cost/benefit of opt-in durability.

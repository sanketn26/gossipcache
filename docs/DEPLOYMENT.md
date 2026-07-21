# Deployment

**Semantics:** [SEMANTICS.md](SEMANTICS.md)
**Architecture:** [ARCHITECTURE.md](ARCHITECTURE.md)

v1 deploys **app+L1** processes and a **native L2 hub**. Redis/UDP-as-control-plane recipes are obsolete.

## Topology

```text
App ASG / Deployment (L1 library in each process)
        │ RPC :7400          │ streams :7401
        └──────────► L2 hub (partitions internal; HA as hub)
```

| Port | Use |
|-----:|-----|
| 7400 | L2 data RPC (mTLS) |
| 7401 | Invalidation streams / checkpoints (mTLS) |
| 8081 | `/livez` `/startupz` `/readyz` (restricted) |
| 9090 | Metrics |

No UDP for backed mode.

## Kubernetes (primary)

| Workload | Kind | Notes |
|----------|------|--------|
| L2 | StatefulSet + PVC | Journal durability; scale shards inside hub |
| App + L1 | Deployment | Link L1 library; no cache sidecar required |

Probes (management port):

- `startupProbe` → `/startupz`
- `livenessProbe` → `/livez` (no L2 dependency)
- `readinessProbe` → `/readyz` (freshness + no unreconciled gaps)

Drain: fail readiness → stop writes → drain → exit.

## Docker (dev)

Hub + two app containers; apps point `L2_ADDRESSES` at hub. No Redis.

## EC2

- App ASG with L1; separate L2 instances + disks
- SG: TCP 7400/7401 between app and hub
- Discovery: tags or static hub list

## Config sketch

```bash
GOSSIPCACHE_L2_ADDRESSES=l2:7400
GOSSIPCACHE_STREAM_FRESHNESS_TIMEOUT=3s
GOSSIPCACHE_STALE_POLICY=never
GOSSIPCACHE_DEFAULT_WRITE_W=0
GOSSIPCACHE_MGMT_LISTEN=127.0.0.1:8081
```

## Troubleshooting

| Symptom | Check |
|---------|--------|
| `STREAM_FRESHNESS_UNKNOWN` | Hub stream, :7401, subscription lease |
| `RECONCILIATION_REQUIRED` | Replay window, anti-entropy |
| Sync/W timeout | Peers ready; lower W or raise timeout; hub commit may already have succeeded |
| Always miss | L1 not linked / wrong config |

## Pre-hybrid history

Older Redis-as-SoT + UDP gossip docs are non-normative. See git history if needed.

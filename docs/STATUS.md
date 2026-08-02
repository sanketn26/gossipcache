# Implementation Status

Single source of truth for **what is built**. Design docs describe the target;
a feature is only real if it appears under **Implemented**.

_Last updated: 2026-08-02_

## Target (v1)

Authoritative product rules: **[SEMANTICS.md](SEMANTICS.md)** — hybrid **L1 +
native L2 hub**. Implementation is tracked in independent Common, Hub and Node
files per phase under [impl/](impl/README.md).

| In scope | Out of scope for v1 |
|----------|---------------------|
| Embedded L1, memory-first L2 Hub as runtime authority | Redis/Postgres as version authority |
| Opt-in Hub durability profile | Mandatory disk dependency for default mode |
| Per-write Fast or Sync acknowledgement, independent of W | Treating peer confirmation as disk durability |
| mTLS **gRPC** Data + Control services | UDP gossip / memberlist control plane |
| VersionTag `(partition_id, sequence)` + `hub_generation` | Independent full-value gossip mode |
| Tunable W (default 0), stale-serve, consistency readiness | Custom RUDP; hand-rolled binary frames |

## Implemented (common contracts — partial P0–P3)

Useful building blocks; **not** a hybrid cluster yet.

| Area | Location | Notes |
|------|----------|--------|
| Shared domain contracts | `internal/wire` | Versions, mutation IDs, bounded requests, write/storage modes, statuses, protocol compatibility and byte-copy rules |
| Partition routing | `internal/wire` | Seeded xxHash64 routing with shared golden vectors |
| P1 Node/Hub seam | `internal/l1` | `HubClient`, validated Get response shapes, value/tombstone kinds and deterministic fake-hook contracts; no Node state machine or Hub fake yet |
| gRPC / protobuf schema | `api/proto/gossipcache/v1`, `api/gen/gossipcache/v1` | `Data` + `Control` services; regenerate with `make proto` |
| P2 control helpers | `internal/control` | Domain types, delivery-rule validation, protobuf conversion; no stream origin or consumer yet |
| P3 data helpers | `internal/rpc` | Domain Get/Mutate/Handshake models, status helpers, dedup fingerprints, protobuf conversion; no gRPC server/client runtime, Hub store, or WAL yet |

## Not started (by phase)

| Phase | Work |
|------:|------|
| P0 remainder | Public facade `New(cfg)`, in-memory L2 fake + basic L1↔backend path |
| P1 | Hub fake implementation plus L1 state machine (EMPTY/FETCHING/VALID/STALE), singleflight and invalidation application |
| P2 | Hub gRPC Control origin and Node consumer: partition streams, interest, replay retention, checkpoint freshness and W aggregation (protobuf schema + domain helpers exist) |
| P3 remainder | Memory Hub store + gRPC Data server; Node gRPC client; opt-in durability/recovery profile and WAL fixtures |
| P4 | Health/readiness, held-key anti-entropy, K8s manifests, min metrics hooks |
| P5 | Full observability suite |
| P6 | Security (mTLS production path on gRPC) |
| P7 | Multi-process demo + polish |
| P8 | Performance optims after baselines |

## Removed / non-v1

- **`internal/backingstore` + Redis adapter** — removed; Redis-as-SoT is a SEMANTICS non-goal.
- Config fields for **UDP gossip**, **independent mode**, and **memberlist-era** network ports — removed in favor of L2 hub settings.
- **Custom binary frame stack** — `internal/frame`, hand-rolled control/RPC codecs, magics `GCS1`/`GCR1`, header CRC32C, hop-ack message, golden hex vectors — deleted; replaced by gRPC + protobuf (`api/proto`).
- **Empty legacy packages** — `internal/api`, `internal/conflict`, `internal/gossip`, `internal/network`, `internal/util`, `internal/vclock` (unused placeholders) — deleted.

Historical ADRs (memberlist, Redis-era evict-on-notify) remain under `docs/adr/` as history; they do not define v1.

## Known debt

- Hub and Node runtime packages do not exist yet; their P0 foundations must
  consume shared `internal/wire` contracts and gRPC stubs without redefining
  them.
Prefer **SEMANTICS** and the matching phase files when any older doc conflicts.

# Implementation phase index

Product behavior is defined by [SEMANTICS.md](../SEMANTICS.md). Implementation
work is maintained only in the phase files below. Each phase has one shared
contract, one Hub plan and one Node plan.

**Wire transport (locked):** mTLS **gRPC + protobuf**. Schema lives in
[`api/proto/gossipcache/v1/`](../../api/proto/gossipcache/v1/). Generated stubs
are under `api/gen/`. There is **no** custom binary frame codec, magic header,
or hop-ack message in v1.

| Phase | Common contract | Hub implementation | Node implementation |
|------:|-----------------|--------------------|---------------------|
| P0 | [Identity, routing, API](common/COMMON_PHASE_00_CONTRACTS.md) | [Foundation](hub/HUB_PHASE_00_FOUNDATION.md) | [Foundation](node/NODE_PHASE_00_FOUNDATION.md) |
| P1 | [State-machine test contract](common/COMMON_PHASE_01_TEST_CONTRACT.md) | [Node-SM support](hub/HUB_PHASE_01_NODE_SM_SUPPORT.md) | [Local state machine](node/NODE_PHASE_01_STATE_MACHINE.md) |
| P2 | [Control protocol (gRPC stream)](common/COMMON_PHASE_02_CONTROL_PROTOCOL.md) | [Stream origin](hub/HUB_PHASE_02_CONTROL_ORIGIN.md) | [Stream consumer](node/NODE_PHASE_02_STREAM_CONSUMER.md) |
| P3 | [Data RPC (gRPC unary)](common/COMMON_PHASE_03_DATA_PROTOCOL.md) | [Memory store + opt-in durability](hub/HUB_PHASE_03_STORAGE.md) | [Real gRPC client](node/NODE_PHASE_03_RPC_CLIENT.md) |
| P4 | [Operations contract](common/COMMON_PHASE_04_OPERATIONS.md) | [Hub operations](hub/HUB_PHASE_04_OPERATIONS.md) | [Node operations](node/NODE_PHASE_04_OPERATIONS.md) |
| P5 | [Observability contract](common/COMMON_PHASE_05_OBSERVABILITY.md) | [Hub observability](hub/HUB_PHASE_05_OBSERVABILITY.md) | [Node observability](node/NODE_PHASE_05_OBSERVABILITY.md) |
| P6 | [Security contract](common/COMMON_PHASE_06_SECURITY.md) | [Hub security](hub/HUB_PHASE_06_SECURITY.md) | [Node security](node/NODE_PHASE_06_SECURITY.md) |
| P7 | [Demo contract](common/COMMON_PHASE_07_DEMO.md) | [Hub packaging](hub/HUB_PHASE_07_DEMO.md) | [Node demo](node/NODE_PHASE_07_DEMO.md) |
| P8 | [Performance contract](common/COMMON_PHASE_08_PERFORMANCE.md) | [Hub performance](hub/HUB_PHASE_08_PERFORMANCE.md) | [Node performance](node/NODE_PHASE_08_PERFORMANCE.md) |

```text
P0 -> P1 -> P2 -> P3 -> P4 -> P5 -> P8
                    \          \
                     P6         P7
```

## Cross-track build order

Each phase lands its Common contract first, then Hub and Node implementations
can proceed in parallel against that contract. A phase exits only when its
cross-component tests pass.

| Phase | Integration seam and dependency |
|------:|---------------------------------|
| P0 | Shared domain types (`internal/wire`), fake Hub, Node facade and Hub memory skeleton |
| P1 | Node state machine uses the deterministic P0 fake Hub; Hub adds precise Get semantics |
| P2 | Stream origin publishes over gRPC `Control.Connect`; does not wait for durable store |
| P3 | Real gRPC `Data` service + memory/durable profiles without changing P1/P2 contracts |
| P4 | Requires running P3 Hub/Node paths; adds reconciliation, readiness and minimum safety instruments |
| P5 | Requires P4 signals; completes exporters, dashboards, alerts and validation |
| P6 | May start after P3 gRPC services freeze; production exit follows P4 readiness integration |
| P7 | May start after P4; final demo consumes P5/P6 when those profiles are enabled |
| P8 | Runs after P5 baselines and all correctness/fault gates |

Hub P2 publishes through an injected `CommitEventSource` implemented by the P0
fake during P2 tests and by the P3 partition commit path later. This makes
“publish after commit eligibility” testable without coupling control delivery
to the storage implementation.

## Ownership

- `common/`: domain + gRPC API contracts and cross-component exit tests.
- `hub/`: L2 authority process work only.
- `node/`: embedded L1 library work only.

Do not put Hub-only storage details in Node files or Node-only state-machine
details in Hub files. Shared types and protobuf live under Common ownership
(`internal/wire`, `api/proto`, conversion helpers in `internal/rpc` /
`internal/control`).

## Package layout (target)

Phase files reference these packages. See [STATUS.md](../STATUS.md) for what
exists today.

| Package | Owner | Contents |
|---------|-------|----------|
| `pkg/gossipcache` | Node | Public `Client` facade, `WriteOptions`, sentinel errors |
| `internal/l1` | Node | Slot state machine, singleflight, ceiling tracking, stale policy |
| `api/proto`, `api/gen` | both | gRPC/protobuf schema and generated stubs (`make proto`) |
| `internal/control` | both | Control-plane domain validation + proto conversion; origin (Hub) / consumer (Node) |
| `internal/rpc` | both | Data-plane domain models, status mapping, dedup, proto conversion; client (Node) / server (Hub) |
| `internal/l2` | Hub | Memory table, version assignment, partition router, changefeed |
| `internal/l2/durable` | Hub | `DurabilityStore`, WAL, recovery, persistence queue |
| `internal/wire` | both | Shared domain types, partition hash golden vectors, `VersionTag` |
| `internal/health` | both | Ready-reason composition and management handlers |
| `internal/antientropy` | both | Held-key digest messages and reconciliation coordination |
| `internal/obs` | both | No-op-first meter, tracer and bounded event interfaces |
| `cmd/l2` | Hub | Hub binary; config, gRPC listeners, lifecycle |
| `test/helpers` | tests | Deterministic clocks, fake Hub hooks and protocol fixtures |
| `test/integration` | tests | Multi-process Hub/Node contract scenarios |
| `test/chaos` | tests | Opt-in crash, gap, stall and persistence-fault tests |
| `deployments/k8s` | ops | Hub/Node manifests, probes and disruption settings |
| `deployments/observability` | ops | Dashboards, recording rules, alerts and runbook links |

### Mergeable file units (P0–P4)

| Owner | Planned units |
|-------|---------------|
| Common | `api/proto/gossipcache/v1/*.proto`, `internal/wire/{types,record,partition,status}.go`, `internal/rpc/{types,convert,dedup,status}.go`, `internal/control/{types,convert}.go`, `internal/health/reason.go`, `internal/antientropy/messages.go` |
| Hub | `internal/l2/{partition,commit,table,expiry,dedup}.go`, `internal/l2/durable/{store,wal,recovery,queue}.go`, `cmd/l2/{main,config,grpc}.go` |
| Node | `pkg/gossipcache/{client,options,errors}.go`, `internal/l1/{slot,machine,fetch,apply,lifecycle}.go`, `internal/rpc/client.go`, `internal/control/consumer.go` |
| Tests/ops | `test/helpers/`, `test/integration/`, `test/chaos/`, `deployments/k8s/`, `deployments/observability/` |

## Detail conventions

Each phase file carries an **Implementation detail** section: concrete package
paths, Go signatures, data structures, algorithms, protobuf/gRPC surfaces,
constants and defaults. Signatures are sketches — SEMANTICS and the P0 `common`
contract win on any conflict. Types shared across components are defined once
in a `common` phase, `internal/wire`, or `.proto`, and referenced (not
redefined) by Hub and Node files.

Concurrency notation: **owned loop** = a single goroutine that owns a
structure; other goroutines communicate via channels or request RPCs.

## Transport summary

| Plane | gRPC service | Pattern | Default port |
|-------|--------------|---------|--------------|
| Data | `gossipcache.v1.Data` | Unary (`Handshake`, `HubStatus`, `Get`, `Mutate`) | `7400` |
| Control | `gossipcache.v1.Control` | Bidi stream `Connect` | **same** listener `7400` (no split port in v1) |
| Management | HTTP | `/livez`, `/startupz`, `/readyz` | `8081` (pod-local) |

Control messages are capped at `MaxControlMessageBytes` (3 MiB) under the
default 4 MiB gRPC message limit. Every Data Get/Mutate carries
`hub_generation` so mismatches fail before commit.

Application-level `Status` enums ride response messages. gRPC status codes are
for transport, auth, and cancellation only.

## Testing expectations (all phases)

| Kind | Scope |
|------|-------|
| Unit (most) | State transitions, routing, proto conversion, commit ordering, W and readiness logic; no real network |
| Package race | Any shared maps, streams, queues, close paths — `go test -race` |
| Integration | One Hub + ≥2 Nodes over real gRPC when P2/P3 land |
| Fault/chaos | Dropped stream messages, stopped checkpoints, crash during Fast/Sync, apply-before-ack races and persistence degradation |

Cross-component scenarios that must stay green once implemented:

| Scenario | Expected |
|----------|----------|
| Local hit | No network; sub-ms p99 objective |
| Miss + singleflight | One Hub Get for concurrent waiters |
| Write W=0 | Commit + local install; peers invalidate async |
| Write W=1 | Waits for one distinct peer apply or returns committed W timeout |
| Gap past retention | Not ready until held-key reconcile |
| Memory Hub restart | New `hub_generation`; Nodes revalidate |
| Drop stream messages | Gap triggers replay or reconciliation; never silently ready |

Phase-specific verification bullets live in each phase file.

| Phase | Extra focus |
|------:|-------------|
| P2 | Disconnect, replay, checkpoint freshness and W tests over gRPC Control |
| P3 | Fake vs real Data client suite; Fast/Sync × W matrix |
| P4 | Readiness reasons, anti-entropy bounds |
| P5–P8 | As listed in those contracts |

Regenerate stubs after proto changes: `make proto`.

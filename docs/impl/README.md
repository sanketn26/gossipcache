# Implementation phase index

Product behavior is defined by [SEMANTICS.md](../SEMANTICS.md). Implementation
work is maintained only in the phase files below. Each phase has one shared
contract, one Hub plan and one Node plan.

| Phase | Common contract | Hub implementation | Node implementation |
|------:|-----------------|--------------------|---------------------|
| P0 | [Identity, routing, API](common/COMMON_PHASE_00_CONTRACTS.md) | [Foundation](hub/HUB_PHASE_00_FOUNDATION.md) | [Foundation](node/NODE_PHASE_00_FOUNDATION.md) |
| P1 | [State-machine test contract](common/COMMON_PHASE_01_TEST_CONTRACT.md) | [Node-SM support](hub/HUB_PHASE_01_NODE_SM_SUPPORT.md) | [Local state machine](node/NODE_PHASE_01_STATE_MACHINE.md) |
| P2 | [Control protocol](common/COMMON_PHASE_02_CONTROL_PROTOCOL.md) | [Stream origin](hub/HUB_PHASE_02_CONTROL_ORIGIN.md) | [Stream consumer](node/NODE_PHASE_02_STREAM_CONSUMER.md) |
| P3 | [Data RPC/storage profiles](common/COMMON_PHASE_03_DATA_PROTOCOL.md) | [Memory store + opt-in durability](hub/HUB_PHASE_03_STORAGE.md) | [Real RPC client](node/NODE_PHASE_03_RPC_CLIENT.md) |
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
| P0 | Shared wire/API types, fake Hub, Node facade and Hub memory skeleton |
| P1 | Node state machine uses the deterministic P0 fake Hub; Hub adds precise Get semantics |
| P2 | Stream origin consumes the P0/P1 commit-event seam; it does not wait for the real P3 RPC or durable store |
| P3 | Replaces fake transport/store with real RPC and memory/durable profiles without changing P1/P2 contracts |
| P4 | Requires running P3 Hub/Node paths; adds reconciliation, readiness and minimum safety instruments |
| P5 | Requires P4 signals; completes exporters, dashboards, alerts and validation |
| P6 | May start after P3 wire protocols freeze; production exit follows P4 readiness integration |
| P7 | May start after P4; final demo consumes P5/P6 when those profiles are enabled |
| P8 | Runs after P5 baselines and all correctness/fault gates |

Hub P2 publishes through an injected `CommitEventSource` implemented by the P0
fake during P2 tests and by the P3 partition commit path later. This makes
“publish after commit eligibility” testable without coupling control framing to
the storage implementation.

## Ownership

- `common/`: wire/API contracts and cross-component exit tests.
- `hub/`: memory-first runtime authority implementation only.
- `node/`: embedded local-cache implementation only.

Update [STATUS.md](../STATUS.md) only after a checked item exists in code. Do
not duplicate a contract in Hub and Node files; link to its `common` phase file.

**v1 scope reminders (do not reintroduce):** subscription leases and
subscriber-to-subscriber relay/peer fanout are not v1 — delivery is Hub → direct
subscribers with connection/checkpoint freshness. Multi-replica Hub HA is
post-v1. See [SEMANTICS.md](../SEMANTICS.md) §4, §9 and §12.

## Package layout (target)

Phase files reference these packages. The tree replaces today's
`internal/cache` + `internal/storage` during P0–P1 (see [STATUS.md](../STATUS.md)
known debt).

| Package | Owner | Contents |
|---------|-------|----------|
| `pkg/gossipcache` | Node | Public `Client` facade, `WriteOptions`, sentinel errors |
| `internal/l1` | Node | Slot state machine, singleflight, ceiling tracking, stale policy |
| `internal/control` | both | Frame codec, stream consumer (Node) and stream origin (Hub) |
| `internal/rpc` | both | Data-plane RPC transport, wire codec, status mapping |
| `internal/l2` | Hub | Memory table, version assignment, partition router, changefeed |
| `internal/l2/durable` | Hub | `DurabilityStore`, WAL, recovery, persistence queue |
| `internal/wire` | both | Shared encoding, golden vectors, `VersionTag`, partition hash |
| `internal/health` | both | Ready-reason composition and management handlers |
| `internal/antientropy` | both | Held-key digest codec and reconciliation coordination |
| `internal/obs` | both | No-op-first meter, tracer and bounded event interfaces |
| `cmd/l2` | Hub | Hub binary; config, listeners, lifecycle |
| `test/helpers` | tests | Deterministic clocks, fake Hub hooks and protocol fixtures |
| `test/integration` | tests | Multi-process Hub/Node contract scenarios |
| `test/chaos` | tests | Opt-in crash, gap, stall and persistence-fault tests |
| `deployments/k8s` | ops | Hub/Node manifests, probes and disruption settings |
| `deployments/observability` | ops | Dashboards, recording rules, alerts and runbook links |

### Mergeable file units (P0–P4)

Exact names may change during implementation, but each row is an independently
reviewable unit with tests; phases must update this table when they choose a
different concrete layout.

| Owner | Planned units |
|-------|---------------|
| Common | `internal/wire/{types,partition,status}.go`, `internal/rpc/{frame,codec}.go`, `internal/control/{frame,messages}.go`, `internal/health/reason.go`, `internal/antientropy/messages.go` |
| Hub | `internal/l2/{partition,commit,table,expiry,dedup}.go`, `internal/l2/durable/{store,wal,recovery,queue}.go`, `cmd/l2/{main,config}.go` |
| Node | `pkg/gossipcache/{client,options,errors}.go`, `internal/l1/{slot,machine,fetch,apply,lifecycle}.go`, `internal/rpc/client.go`, `internal/control/consumer.go` |
| Tests/ops | `test/helpers/`, `test/integration/`, `test/chaos/`, `deployments/k8s/`, `deployments/observability/` |

## Detail conventions

Each phase file carries an **Implementation detail** section: concrete package
paths, Go signatures, data structures, algorithms, wire layout, constants and
defaults. Signatures are sketches — SEMANTICS and the P0 `common` contract win
on any conflict. Types shared across components are defined once in a `common`
phase or `internal/wire`, and referenced (not redefined) by Hub and Node files.

Concurrency notation used below: **owned loop** = a single goroutine that owns a
state region; **fenced** = a synchronous durability boundary; **watermark** = the
highest contiguous applied/persisted sequence for a partition.

## Test strategy and phase gates

Tests stay next to their owning phase contract instead of duplicating behavior
in a separate monolith.

| Layer | Scope |
|-------|-------|
| Unit (most) | State transitions, routing, codecs, commit ordering, W and readiness logic; no real network |
| Integration | One Hub and at least two Nodes: W=0/W=1, concurrent miss, replay, restart and generation change |
| Fault/chaos | Dropped frames, stopped checkpoints, crash during Fast/Sync, hop-ack-before-apply and persistence degradation |
| Bench (gated) | Local hit, miss, Fast/Sync, fanout, replay and recovery under Common P8 profile |

Coverage target is **greater than 80%** for `internal/l1`, `internal/control`
and the Hub partition commit path. Coverage is a diagnostic target, not a reason
to weaken assertions or test implementation trivia.

Minimum integration cases:

| Case | Expected result |
|------|-----------------|
| Write on A, read on A | New value immediately from local install |
| Fast/W=0 write on A, immediate read on B | Old value is temporarily permitted |
| Fast/W=0 write, wait for stream apply, read on B | B fetches/returns the new version |
| W=1 write | Waits for one distinct peer apply or returns committed W timeout |
| Concurrent cold reads | One Hub Get per key through singleflight |
| Memory Hub restart | New generation, empty Hub, Nodes revalidate before ready |
| Durable restart after Sync | Sync-acknowledged value and contiguous stream/version heads recover |
| Node restart | Starts empty, re-subscribes and demand-fills |

Minimum deterministic fault cases:

| Fault | Expected result |
|-------|-----------------|
| Drop stream frames | Gap triggers replay or reconciliation; never silently ready |
| Stop checkpoints | `STREAM_FRESHNESS_UNKNOWN` and reconnect/replay |
| Hub unavailable | Writes fail; misses follow the configured stale policy |
| Crash during Fast tail | New generation if acknowledged tail may be lost |
| Crash during Sync fence | No Sync success without recoverable value/version/event prefix |
| Node stops after hop ack but before apply | Application watermark does not advance |
| W>0 with insufficient peers | `ErrWriteConfirmTimeout`; committed mutation remains |
| Telemetry exporter stalls | Get, commit and apply continue; bounded drop signal increments |

| Gate | Required evidence |
|------|-------------------|
| P0 | Shared types/golden routing vectors; fake Hub and Node facade interoperate |
| P1 | Exhaustive state/event table and race tests |
| P2 | Disconnect, replay, checkpoint freshness and W tests |
| P3 | Both profiles; Fast/Sync crash and recovery; no sequence holes while generation is unchanged |
| P4 | Ready never true with a gap, stale checkpoint, recovery or reconciliation pending |
| P5 | Fault-to-signal matrix, cardinality CI and exporter-failure isolation |
| P6 | Wrong CA/SAN/cluster/generation and expired credentials fail closed |
| P7 | Scripted demos assert convergence and expected failure states |
| P8 | Reproducible before/after benchmarks with all correctness gates green |

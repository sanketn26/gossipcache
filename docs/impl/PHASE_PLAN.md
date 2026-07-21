# Phased Implementation Plan (v1 hybrid) — code-level

**Product rules:** [../SEMANTICS.md](../SEMANTICS.md) (wins on conflict)
**Wire/algorithms:** [HYBRID_BACKED_MODE.md](HYBRID_BACKED_MODE.md)
**Tests:** [TESTING_STRATEGY.md](TESTING_STRATEGY.md)
**Observability detail:** [PHASE_5_OBSERVABILITY.md](PHASE_5_OBSERVABILITY.md)
**API sketch:** [../TECHNICAL_SPEC.md](../TECHNICAL_SPEC.md)

Module: `github.com/sanketn26/gossipcache` (`go.mod` already present).
Empty dirs under `internal/` today are **placeholders** — reorganize to the tree below; do not fill `backingstore/`, `vclock/`, or Redis paths for v1.

Each **functional deliverable (FD)** is a mergeable unit: code + tests + checklist.

---

## Target package layout

```text
gossipcache/
├── go.mod
├── cmd/
│   ├── l2/main.go                 # hub process
│   └── gossipcache-demo/main.go   # optional demo app+L1 (P2+)
├── pkg/gossipcache/
│   ├── gossipcache.go             # New(cfg), public facade
│   ├── types.go                   # VersionTag, WriteOptions, errors
│   ├── config.go                  # public Config (or re-export)
│   └── doc.go
├── internal/
│   ├── version/
│   │   ├── tag.go                 # VersionTag, Compare, Less
│   │   └── partition.go           # PartitionID(key, shardCount) hash
│   ├── l1/
│   │   ├── cache.go               # Cache orchestrator Get/Set/Delete
│   │   ├── slot.go                # per-key Slot + mutex + state
│   │   ├── machine.go             # transition functions (pure-ish)
│   │   ├── record.go              # immutable Record {Value, Tag, Expiry}
│   │   ├── singleflight.go        # or wrap golang.org/x/sync/singleflight
│   │   ├── apply.go               # ApplyInvalidation
│   │   ├── policy.go              # StalePolicy
│   │   └── *_test.go
│   ├── l2/
│   │   ├── api.go                 # shared RPC request/response types
│   │   ├── client/
│   │   │   ├── client.go          # L2Client interface + mTLS RPC
│   │   │   └── mem.go             # in-memory fake for unit tests
│   │   └── server/
│   │       ├── server.go          # RPC server
│   │       ├── shard.go           # one shard: map + seq + journal handle
│   │       ├── commit.go          # atomic value+tag+changefeed
│   │       ├── journal.go         # WAL append/replay
│   │       ├── changefeed.go      # stream_sequence + replay log
│   │       ├── subscribe.go       # stream subscribers
│   │       └── engine.go          # multi-shard router
│   ├── control/
│   │   ├── frame.go               # length-prefix codec
│   │   ├── messages.go            # InvalidationBatch, HopAck, StreamAck, Checkpoint, Confirm
│   │   ├── stream_client.go       # L1: subscribe, apply, watermarks
│   │   ├── stream_server.go       # hub: push, replay, checkpoints
│   │   ├── watermark.go           # contiguous applied seq per stream
│   │   ├── freshness.go           # checkpoint age → STREAM_FRESHNESS_UNKNOWN
│   │   ├── w.go                   # wait for W confirms
│   │   └── *_test.go
│   ├── health/
│   │   ├── evaluator.go           # shared readiness reasons
│   │   └── http.go                # /livez /startupz /readyz
│   ├── antientropy/
│   │   └── heldkeys.go            # compare held keys vs hub digest
│   ├── config/
│   │   └── config.go              # load env/file → Config
│   ├── obs/
│   │   ├── meter.go               # Meter interface + noop
│   │   ├── names.go               # metric name constants
│   │   └── prometheus.go          # P5 exporter
│   └── util/
│       └── clock.go               # injectable time for tests
├── test/
│   ├── helpers/                   # start hub, start L1, wait ready
│   ├── integration/
│   └── chaos/
└── deployments/k8s/               # P4 manifests
```

**Deprecate / leave empty for v1:** `internal/backingstore`, `internal/vclock`, `internal/gossip` (old meaning), `internal/conflict`. Prefer deleting or adding `//go:build ignore` README so they are not mistaken for the path.

---

## Shared types (land in P0, used everywhere)

```go
// pkg/gossipcache/types.go (or internal/version exported via facade)
type VersionTag struct {
    PartitionID uint32
    Sequence    uint64
}

func (a VersionTag) Less(b VersionTag) bool  // same PartitionID only; else not comparable
func (a VersionTag) Equal(b VersionTag) bool

type KeyState uint8
const (
    StateEmpty KeyState = iota
    StateFetching
    StateValid
    StateStale
)

type StalePolicy uint8
const (
    StaleNever StalePolicy = iota
    StaleIfError
    ServeStaleWhileRevalidate
)

type WriteOptions struct {
    W            int
    ConfirmLevel ConfirmLevel
    Timeout      time.Duration
}

type ConfirmLevel uint8
const (
    ConfirmInvalidateApplied ConfirmLevel = iota
)

// Sentinel errors
var (
    ErrNotFound            = errors.New("not found")
    ErrNotCaughtUp         = errors.New("not caught up") // L2: retryable
    ErrWriteConfirmTimeout = errors.New("write confirm timeout")
    ErrHubUnavailable      = errors.New("hub unavailable")
)
```

```go
// internal/l2/api.go — RPC messages (protobuf later; Go structs + gob/json OK for P0–P2)
type GetRequest struct {
    Key            []byte
    MinVersion     *VersionTag // optional ceiling
}
type GetResponse struct {
    Found     bool
    Value     []byte
    Version   VersionTag
    ExpiryUnixNano int64
    Tombstone bool
    Code      StatusCode // OK, NOT_FOUND, NOT_CAUGHT_UP, ERROR
}

type SetRequest struct {
    Key      []byte
    Value    []byte
    TTL      time.Duration
    W        int           // 0 default; hub-aggregated peer confirms
    WTimeout time.Duration // hub-enforced when W > 0
}
type SetResponse struct {
    Version VersionTag
}

type DeleteRequest struct { Key []byte }
type DeleteResponse struct {
    Version VersionTag
}
```

```go
// internal/control/messages.go
type StreamID struct {
    PartitionID uint32
}

type InvalidationEvent struct {
    Op             Op // Invalidate | Delete
    Key            []byte
    Version        VersionTag
    StreamSeq      uint64
}

type InvalidationBatch struct {
    StreamID StreamID
    FirstSeq uint64
    Events   []InvalidationEvent
}

type StreamAcknowledgement struct {
    NodeID              uint64
    StreamID            StreamID
    ContiguousAppliedSeq uint64
    Missing             []SeqRange
}

type InvalidateConfirm struct { // used for W > 0
    NodeID  uint64
    Key     []byte
    Version VersionTag
    StreamSeq uint64
}

type StreamCheckpoint struct {
    StreamID   StreamID
    HeadSeq    uint64
    HubGen     uint64
    WallNanos  int64
}
```

---

## P0 — Foundation

**Goal:** Buildable module, public API, single-process memory cache, injectable L2 port.

### FD-P0.1 — Module skeleton and public facade

| | |
|--|--|
| **Files** | `pkg/gossipcache/{doc.go,gossipcache.go,types.go}`, `internal/config/config.go`, `internal/obs/meter.go` |
| **Code** | `func New(cfg Config) (*Client, error)`, `Client` embeds/wraps `l1.Cache`; `Start`/`Stop` no-ops or lifecycle hooks; `Meter` interface + `NoopMeter` |
| **Tests** | `TestNew_DefaultConfig` |
| **Done** | `go build ./...`, `go test ./pkg/...` green |

```go
// pkg/gossipcache/gossipcache.go
type Client struct {
    l1 *l1.Cache
    // later: stream, health
}

func New(cfg Config) (*Client, error)
func (c *Client) Get(ctx context.Context, key string) ([]byte, error)
func (c *Client) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
func (c *Client) Delete(ctx context.Context, key string) error
func (c *Client) SetWithOptions(ctx context.Context, key string, val []byte, ttl time.Duration, opt WriteOptions) error
func (c *Client) Start(ctx context.Context) error
func (c *Client) Stop(ctx context.Context) error
```

### FD-P0.2 — Config

| | |
|--|--|
| **Files** | `internal/config/config.go`, `config_test.go` |
| **Code** | `Config` with `L2Addresses`, `ShardCount`, `DefaultTTL`, `DefaultW`, `StalePolicy`, `StreamFreshnessTimeout`, `RPCAddr`, `ControlAddr`, `MgmtListen`, `NodeID`, TLS paths; `LoadFromEnv()` |
| **Defaults** | `DefaultW=0`, `StaleNever`, `ShardCount=16`, freshness `3s` |
| **Done** | Invalid config rejected; env overrides work |

### FD-P0.3 — Version + partition helpers

| | |
|--|--|
| **Files** | `internal/version/{tag.go,partition.go,tag_test.go}` |
| **Code** | `VersionTag`, `Compare`, `Less` (panic or error if partition differs), `PartitionID(key []byte, n uint32) uint32` (e.g. xxhash % n) |
| **Done** | Table tests for order and hash stability |

### FD-P0.4 — In-memory L2 fake + local-only cache path

| | |
|--|--|
| **Files** | `internal/l2/client/mem.go`, `internal/l1/cache.go` (minimal), `internal/l1/record.go` |
| **Code** | `type Backend interface { Get…; Set…; Delete… }` (name: `l2.Store` or `l2client.Client`); `MemStore` with `sync.Mutex`, map, per-partition `seq uint64`; `l1.Cache` Get/Set/Delete calling backend, stores `map[string]*Record` only (no full SM yet) |
| **Done** | Unit: set/get/delete/ttl expire against MemStore |

**P0 exit:** single-process Get/Set/Delete works; **no Redis**; Backend interface ready for real hub.

---

## P1 — L1 state machine

**Goal:** SEMANTICS §5–7 fully in `internal/l1`.

### FD-P1.1 — Slot + immutable record

| | |
|--|--|
| **Files** | `internal/l1/{slot.go,record.go,slot_test.go}` |
| **Code** | |

```go
type Record struct { // immutable after publish
    Value     []byte
    Version   version.VersionTag
    Expiry    time.Time // zero = none
    Tombstone bool
}

type Slot struct {
    mu              sync.Mutex
    state           KeyState
    record          *Record           // queryable when VALID / maybe STALE
    retained        *Record           // for StaleIfError / SWR
    maxInvalidated  *version.VersionTag
    inflight        *flight           // singleflight waiters
}

func (s *Slot) snapshot() (KeyState, *Record, *version.VersionTag)
```

### FD-P1.2 — Transition engine

| | |
|--|--|
| **Files** | `internal/l1/machine.go`, `machine_test.go` |
| **Code** | Pure functions driven by events: |

```go
type Event any // typed events:
// EvRead, EvL2OK{Rec}, EvL2Err{err}, EvL2NotCaughtUp, EvInvalidate{Tag},
// EvWriteOK{Rec}, EvDeleteOK{Tag}, EvTTL, EvEvict

func Apply(s *Slot, e Event, pol StalePolicy, now time.Time) (actions []Action)
// Action: WakeWaiters, StartFetch, ReturnValue, ReturnErr, ...
```

Cover every row in SEMANTICS transition table. Prefer table-driven tests over integration.

### FD-P1.3 — Singleflight fetch

| | |
|--|--|
| **Files** | `internal/l1/singleflight.go`, `cache.go` Get path |
| **Code** | On miss/stale: `state=FETCHING`; one goroutine calls `backend.Get(ctx, key, min)`; waiters block on `chan result`; publish `Record` only if `acceptFetch(rec, maxInvalidated)` |

```go
func acceptFetch(rec *Record, ceiling *VersionTag) bool {
    if ceiling == nil { return true }
    if rec.Version.PartitionID != ceiling.PartitionID { return false }
    return !rec.Version.Less(*ceiling) // >=
}
```

### FD-P1.4 — Write install (read-your-writes)

| | |
|--|--|
| **Files** | `internal/l1/cache.go` Set/Delete |
| **Code** | `ver, err := backend.Set(...)`; on success **before return**: install `Record` → `VALID` under slot lock; bump/clear ceiling so write version not rejected; if `FETCHING`, complete waiters with new record |

### FD-P1.5 — ApplyInvalidation

| | |
|--|--|
| **Files** | `internal/l1/apply.go` |
| **Code** | |

```go
func (c *Cache) ApplyInvalidation(ev control.InvalidationEvent) {
    // NON-ALLOCATING lookup only — do NOT create a slot for unknown keys.
    slot, ok := c.slots.Get(string(ev.Key)) // map lookup; no insert
    if !ok {
        // Not a holder/fetcher: stream client still advances watermark + app ack.
        // No per-key ceiling, no map growth from cluster traffic.
        return
    }
    slot.mu.Lock()
    defer slot.mu.Unlock()
    // VALID + newer → STALE; equal/older → no-op; FETCHING → raise maxInvalidated only
}
```

**Invariant:** subscribed invalidation traffic must not allocate one L1 slot per cluster key.

### FD-P1.6 — Race test suite

| | |
|--|--|
| **Files** | `internal/l1/race_test.go` |
| **Tests** | invalidate during fetch; L2 returns old; writer+get; concurrent get same key (one backend call); tombstone |

**P1 exit:** all SM unit tests green; bench stub `BenchmarkL1Hit` exists.

---

## P2 — Control plane + W

**Goal:** Real invalidation stream from hub process (MemStore or early server) to multiple L1s.

### FD-P2.1 — Frame codec

| | |
|--|--|
| **Files** | `internal/control/frame.go`, `frame_test.go` |
| **Code** | `WriteFrame(w, typ byte, payload []byte)`, `ReadFrame(r)` — 4-byte BE length + 1-byte type + payload; max frame size constant |

### FD-P2.2 — Message encode/decode

| | |
|--|--|
| **Files** | `internal/control/messages.go`, `codec.go` |
| **Code** | Binary or protobuf for `InvalidationBatch`, `StreamAcknowledgement`, `HopFrameAck`, `StreamCheckpoint`, `ReplayRequest`, `InvalidateConfirm`. Golden vector tests. |

### FD-P2.3 — Stream server (hub side)

| | |
|--|--|
| **Files** | `internal/control/stream_server.go`, `internal/l2/server/changefeed.go` (or temporary on MemStore) |
| **Code** | |

```go
type StreamServer struct {
    shards map[uint32]*shardStream // replay ring, subs, headSeq
}

func (s *StreamServer) Publish(partition uint32, ev InvalidationEvent) // assigns StreamSeq, appends replay, push to subs
func (s *StreamServer) ServeConn(conn net.Conn)
func (s *StreamServer) CheckpointLoop(interval time.Duration)
```

Replay: ring buffer per partition `[]InvalidationEvent` + `firstSeq`; `Replay(from,to)`.

### FD-P2.4 — Stream client (L1 side)

| | |
|--|--|
| **Files** | `internal/control/stream_client.go`, `watermark.go`, `freshness.go` |
| **Code** | |

```go
type StreamClient struct {
    watermarks map[uint32]*Watermark // applied contiguous seq
    lastCheckpoint map[uint32]time.Time
    onEvent func(InvalidationEvent)
}

func (c *StreamClient) Subscribe(ctx context.Context, partitions []uint32) error
func (c *StreamClient) Run(ctx context.Context) error // read loop
func (c *StreamClient) FreshnessOK(now time.Time, timeout time.Duration) (bool, Reason)
```

On batch: for each event in order, if gap → `ReplayRequest`; else `cache.ApplyInvalidation`; advance applied watermark; send `StreamAcknowledgement`; if W-tracking, emit `InvalidateConfirm`.

### FD-P2.5 — Interest / auto-subscribe

| | |
|--|--|
| **Files** | `internal/l1/cache.go` + `stream_client` |
| **Code** | On first Get/Set for key → `pid := version.PartitionID(key, shardCount)`; ensure subscribed to `pid` (mutex set of partitions). v1: never auto-unsubscribe. |

### FD-P2.6 — Tunable W (hub-aggregated; no writer-local race)

**Architecture (locked, SEMANTICS §8):** W is completed **inside the Set/Delete RPC by the hub**, not by a waiter registered on the writer after return.

| | |
|--|--|
| **Files** | `internal/l2/server/w_confirm.go`, `internal/l2/api.go` SetRequest, `internal/control` confirm msg, `l1/cache.go` SetWithOptions, `l2/client` |
| **Code** | |

```go
// SetRequest includes W (0 default).
type SetRequest struct {
    Key, Value []byte
    TTL        time.Duration
    W          int           // peer confirms required; 0 = return after durable commit
    WTimeout   time.Duration // hub-enforced
}

// Hub Set RPC handler (pseudo):
func (s *Server) Set(ctx context.Context, req SetRequest) (SetResponse, error) {
    tag, streamSeq, err := s.engine.CommitSet(...) // durable first
    if err != nil { return SetResponse{}, err }
    s.feed.Publish(event) // invalidation eligible immediately after commit

    if req.W <= 0 {
        return SetResponse{Version: tag}, nil
    }
    // Block RPC until W distinct peer node_ids confirm this streamSeq/key/version
    // or WTimeout. Dedup map[nodeID]struct{}. Writer node_id excluded.
    if err := s.waitConfirms(ctx, tag, streamSeq, req.W, req.WTimeout); err != nil {
        // Commit stands; client may still install from partial response policy:
        // v1: return error ErrWriteConfirmTimeout with Version in error detail
        // so writer can install locally if desired, or client always installs
        // only on full OK — pick one: v1 = return tag + error? cleaner:
        // return (tag, ErrWriteConfirmTimeout) via custom result type
        return SetResponse{Version: tag}, ErrWriteConfirmTimeout
    }
    return SetResponse{Version: tag}, nil
}

// L1 peer after ApplyInvalidation (slot existed or not — confirm is for event):
// if event has W-tracking (always for W>0 writes): send InvalidateConfirm to hub
// hub correlates by (stream_id, stream_seq) or (key, version)

// Writing L1:
resp, err := client.Set(ctx, SetRequest{..., W: opt.W, WTimeout: opt.Timeout})
// On success OR on ErrWriteConfirmTimeout with Version present:
//   install local VALID from resp.Version (RYW) — document:
// v1 rule: install on any durable success including confirm timeout
//          so writer always RYW; error only signals W not met
if resp.Version.Sequence != 0 {
    c.installWrite(key, val, resp.Version, ttl)
}
return err // nil or ErrWriteConfirmTimeout
```

**Rules**

1. **No writer-local `registerWaiter` after commit** — eliminates confirm-before-register race.
2. **Hub is the only aggregator** for W; sequence diagram matches code.
3. **Dedup by confirming `node_id`** — one peer ≤ 1 count.
4. **Commit before wait** — timeout never rolls back journal.
5. Unit test: inject confirm before “would have registered local waiter”; still succeeds.

Default `Set` uses `W=0` (RPC returns immediately after durable commit).

### FD-P2.7 — Integration: two L1 + stream

| | |
|--|--|
| **Files** | `test/integration/invalidate_test.go`, `test/helpers/cluster.go` |
| **Code** | `StartMemHub(t)`, `StartL1(t, hub)`; write on A W=0; wait until B applies; Get B == new; kill stream mid-way; assert replay or not-ready |

**P2 exit:** multi-L1 invalidation works; freshness + W unit/integration tests green.

---

## P3 — Durable L2 hub

**Goal:** Replace MemStore with durable hub process.

### FD-P3.1 — Journal format (replayable)

| | |
|--|--|
| **Files** | `internal/l2/server/journal.go`, `journal_test.go` |
| **Code** | Append-only, length-delimited records: |

```text
Record layout (little-endian or BE — pick one and test):
  magic        uint32   // e.g. 0x47534A31 "GSJ1"
  version      uint8    // journal schema = 1
  type         uint8    // RecPut | RecDelete | RecStreamMeta | ...
  payload_len  uint32   // length of payload; max e.g. 16MiB config
  payload      bytes    // payload_len bytes
  crc32c       uint32   // over magic..payload inclusive

File: sequential concatenation of records.
Replay:
  - read header; if EOF mid-header → torn tail, stop (truncate optional)
  - if payload_len > max → corrupt, fail closed
  - read payload+crc; CRC fail → torn/corrupt tail, stop before bad record
  - never skip using CRC alone without length
```

API: `Open`, `Append([]JournalRec) error` (single fsync boundary per commit batch), `Replay(fn)`, `TruncateTail()`.

### FD-P3.2 — Shard engine + atomic commit

| | |
|--|--|
| **Files** | `internal/l2/server/{shard.go,commit.go}` |
| **Code** | |

```go
type Shard struct {
    id     uint32
    mu     sync.Mutex
    data   map[string]stored
    nextSeq uint64
    nextStreamSeq uint64
    journal *Journal
    feed    *ChangeFeed
}

func (s *Shard) CommitSet(key, val []byte, ttl time.Duration) (VersionTag, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    // 1) Candidates only — do not mutate durable counters yet
    candSeq := s.nextSeq + 1
    candStream := s.nextStreamSeq + 1
    tag := VersionTag{PartitionID: s.id, Sequence: candSeq}
    recs := buildJournalRecs(key, val, ttl, tag, candStream)

    // 2) Durable barrier
    if err := s.journal.Append(recs); err != nil {
        return VersionTag{}, err // nextSeq/nextStreamSeq unchanged
    }

    // 3) Publish memory + feed only after durable success
    s.nextSeq = candSeq
    s.nextStreamSeq = candStream
    s.data[string(key)] = stored{Value: val, Ver: tag, ...}
    s.feed.Publish(InvalidationEvent{
        Key: key, Version: tag, StreamSeq: candStream, ...
    })
    return tag, nil
}
```

**Invariant:** never bump `nextSeq` / `nextStreamSeq` / `data` / feed before successful append+fsync. On append failure, return error immediately.

### FD-P3.3 — Multi-shard engine + RPC server

| | |
|--|--|
| **Files** | `internal/l2/server/{engine.go,server.go}`, `cmd/l2/main.go` |
| **Code** | `Engine.Route(key) *Shard`; TCP/gRPC server implementing Get/Set/Delete; `cmd/l2` flags: `-data`, `-rpc`, `-control`, `-shards` |

### FD-P3.4 — RPC client (real)

| | |
|--|--|
| **Files** | `internal/l2/client/client.go` |
| **Code** | Dial hub, mTLS optional in P3 (can be plain TCP dev), timeouts, `Get` maps `NOT_CAUGHT_UP` → `ErrNotCaughtUp` |

### FD-P3.5 — Recovery

| | |
|--|--|
| **Files** | `internal/l2/server/recover.go`, `recover_test.go` |
| **Code** | On start: replay journal → rebuild `data`, `nextSeq`, `nextStreamSeq`, changefeed replay buffer; crash tests using `t.TempDir` + kill mid-write |

### FD-P3.6 — Integration durable path

| | |
|--|--|
| **Files** | `test/integration/hub_restart_test.go` |
| **Done** | Set → kill hub → restart → Get returns value; second L1 still invalidates after restart publish |

**P3 exit:** never lose acked write; version never regresses on restart.

---

## P4 — Anti-entropy, health, K8s

### FD-P4.1 — Health evaluator

| | |
|--|--|
| **Files** | `internal/health/{evaluator.go,http.go,reasons.go}` |
| **Code** | |

```go
type Reason string
const (
    ReasonOK Reason = ""
    ReasonReconciliationRequired = "RECONCILIATION_REQUIRED"
    ReasonStreamFreshnessUnknown = "STREAM_FRESHNESS_UNKNOWN"
    ReasonHubUnavailable         = "HUB_UNAVAILABLE"
    // ...
)

type Evaluator struct {
    Stream  FreshnessSource
    Hub     HubPinger
    Recon   func() bool
}

func (e *Evaluator) Ready() (bool, []Reason)
func MountHTTP(mux *http.ServeMux, e *Evaluator, live LiveChecker)
```

Wire into `Client.Start`: listen `MgmtListen`.

### FD-P4.2 — Held-key anti-entropy

| | |
|--|--|
| **Files** | `internal/antientropy/heldkeys.go` |
| **Code** | Periodically: for each resident key, or hub range digest API `Summarize(partition)`; mismatch → `ApplyInvalidation` synthetic ceiling / mark STALE; on expired gap set `ReconciliationRequired=true` until pass |

### FD-P4.3 — K8s manifests

| | |
|--|--|
| **Files** | `deployments/k8s/{l2-statefulset.yaml,app-deployment.example.yaml,services.yaml}` |
| **Content** | probes to 8081; PDB; resource stubs; env from SEMANTICS config names |

### FD-P4.4 — Min metrics hooks

| | |
|--|--|
| **Files** | `internal/obs/names.go`, counters in l1/control/l2 hot paths |
| **Code** | `Meter.Counter(name).Add(1)`; no-op default; optional Prometheus register in P5 |

**P4 exit:** ready never true with gap/stale checkpoint; `deployments/k8s` applies on kind/minikube smoke.

---

## P5 — Observability suite

Break [PHASE_5_OBSERVABILITY.md](PHASE_5_OBSERVABILITY.md) into code FDs:

| FD | Deliverable |
|----|-------------|
| FD-P5.1 | Prometheus exporter + `/metrics` |
| FD-P5.2 | Cardinality allowlist test (CI) |
| FD-P5.3 | OTel tracer hooks on miss/write/replay (not hit) |
| FD-P5.4 | Dashboard JSON + alert rules in `deployments/observability/` |
| FD-P5.5 | Fault-to-signal table test harness |

---

## P6 — Performance (only after baselines)

| FD | Deliverable |
|----|-------------|
| FD-P6.1 | Benchmark suite + published profile (hardware note) |
| FD-P6.2 | Hot-tier / journal batching behind `Config` flags |
| FD-P6.3 | Each optim: feature flag + A/B bench + correctness suite still green |

---

## Dependency graph (functional deliverables)

```text
P0.1 → P0.2 → P0.3 → P0.4
                ↓
        P1.1 → P1.2 → P1.3 → P1.4 → P1.5 → P1.6
                                    ↓
        P2.1 → P2.2 → P2.3 → P2.4 → P2.5 → P2.6 → P2.7
                         ↓
        P3.1 → P3.2 → P3.3 → P3.4 → P3.5 → P3.6
                                    ↓
                    P4.1 → P4.2 → P4.3 → P4.4
                                    ↓
                         P5.* → P6.*
```

---

## Per-FD PR checklist

- [ ] Implements only listed files/behavior
- [ ] Unit tests in package
- [ ] No Redis / UDP control plane
- [ ] SEMANTICS not violated (especially write barrier, W default 0, hub-only publish)
- [ ] `go test ./...` green
- [ ] Update checkbox in this file

---

## Progress

### P0
- [ ] FD-P0.1 Facade
- [ ] FD-P0.2 Config
- [ ] FD-P0.3 Version/partition
- [ ] FD-P0.4 MemStore + basic cache

### P1
- [ ] FD-P1.1 Slot/record
- [ ] FD-P1.2 Machine
- [ ] FD-P1.3 Singleflight
- [ ] FD-P1.4 Write install
- [ ] FD-P1.5 ApplyInvalidation
- [ ] FD-P1.6 Race tests

### P2
- [ ] FD-P2.1 Frames
- [ ] FD-P2.2 Messages
- [ ] FD-P2.3 Stream server
- [ ] FD-P2.4 Stream client
- [ ] FD-P2.5 Interest
- [ ] FD-P2.6 W
- [ ] FD-P2.7 Integration

### P3
- [ ] FD-P3.1 Journal
- [ ] FD-P3.2 Commit
- [ ] FD-P3.3 Engine+RPC+cmd
- [ ] FD-P3.4 Client
- [ ] FD-P3.5 Recovery
- [ ] FD-P3.6 Hub restart integration

### P4
- [ ] FD-P4.1 Health
- [ ] FD-P4.2 Anti-entropy
- [ ] FD-P4.3 K8s
- [ ] FD-P4.4 Min metrics

### P5 / P6
- [ ] As FDs above

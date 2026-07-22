# Hub P0 — Foundation

**Common contract:** [COMMON_PHASE_00_CONTRACTS.md](../common/COMMON_PHASE_00_CONTRACTS.md).

## Outcome

A configured in-memory authority exists for node development without implying
that the node can mint versions.

## Functional work

- [ ] Define hub-only config: storage profile (`memory` default), optional data
  path for durable mode, partition count, generation and RPC, control and
  management listeners.
- [ ] Implement an in-memory hub fake behind the shared hub API.
- [ ] Route keys with shared golden partition vectors.
- [ ] Assign authoritative per-partition versions for Set/Delete.
- [ ] Copy retained and returned mutable bytes.

## Implementation detail

### Packages

- `cmd/l2`: binary entrypoint, flag/env config, listener wiring, lifecycle.
- `internal/l2`: authority core — partition router, per-partition memory table,
  version assignment. In P0 this is a fake exposed behind the shared hub API.

### Config

```go
type HubConfig struct {
    StorageProfile wire.StorageProfile // memory (default) | durable
    DataDir        string              // required iff durable
    PartitionCount uint32              // fixed for a generation; default 16
    HubGeneration  uint64              // minted at start (memory) or recovered (durable)
    RPCListen      string              // :7400
    ControlListen  string              // :7401
    MgmtListen     string              // 127.0.0.1:8081
}
```

Validation: `durable` without a usable `DataDir` is a startup error; `memory`
mints a fresh `HubGeneration` (monotonic clock nanos + random) every start.

### Version authority

The hub is the sole minter of `VersionTag`. Per partition it holds a
`sequence uint64` under a partition mutex; Set/Delete assign
`next = atomic-under-lock(sequence++)` then commit. The node can never mint a
version — the P0 fake enforces this by having no node-side sequence field.

```go
type memPartition struct {
    mu       sync.Mutex
    sequence uint64
    table    map[string]record // key -> {value/tombstone, VersionTag, expiry}
}
```

Retained request bytes and returned response bytes are copied via
`wire.CopyBytes` at the boundary.

## Verification

- [ ] Config validation/default tests.
- [ ] Routing/version monotonicity and byte-aliasing tests.
- [ ] Node contract tests run against the fake.

**Exit:** the fake is the sole version authority and supports deterministic node
tests for Get, Set, Delete and TTL.

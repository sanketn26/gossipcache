# Common P0 — Identity, routing and API contracts

## Shared types

```go
type VersionTag struct {
    PartitionID uint32
    Sequence    uint64
}

type GetRequest struct {
    Key        []byte
    MinVersion *VersionTag
}
```

`VersionTag` is ordered only within one partition. Sequence is strictly
monotonic under the installed `hub_generation`. Keys and values are bytes;
retained/returned mutable bytes are copied at boundaries.

## Decisions

- [ ] Freeze hash algorithm, seed, byte interpretation and modulo behavior.
- [ ] Publish golden key-to-partition vectors.
- [ ] Freeze bounded Get/Set/Delete models, TTL units and status codes.
- [ ] Define stable mutation request ID shape.
- [ ] Define `WriteMode` with `WriteFast` default and explicit `WriteSync`, kept
  independent from peer-confirmation `W`.
- [ ] Define protocol compatibility/version fields.
- [ ] Define `memory` as the default Hub storage profile and reject durable mode
  without valid persistence configuration.

## Implementation detail

### Shared package (`internal/wire`)

All P0 shared types live in `internal/wire` so Hub and Node import one
definition. Neither component redefines them.

```go
type VersionTag struct {
    PartitionID uint32
    Sequence    uint64 // strictly monotonic within (PartitionID, hub_generation)
}

// IsNewer reports whether v supersedes other. Only comparable within the same
// partition and generation; the caller checks generation separately.
func (v VersionTag) IsNewer(other VersionTag) bool {
    return v.PartitionID == other.PartitionID && v.Sequence > other.Sequence
}

type WriteMode uint8
const (
    WriteFast WriteMode = iota // ack on atomic hub memory commit (default, zero value)
    WriteSync                  // ack after synchronous durability fence
)

type StorageProfile uint8
const (
    StorageMemory  StorageProfile = iota // ephemeral, default
    StorageDurable                       // opt-in persistence
)

type ConfirmLevel uint8
const (
    ConfirmInvalidateApplied ConfirmLevel = iota // only v1 confirmation level
)

type WriteOptions struct {
    W            uint16
    Mode         WriteMode
    ConfirmLevel ConfirmLevel
    Timeout      time.Duration
}
```

`WriteFast` and `StorageMemory` are the zero values so an unset request is the
default path. `ConfirmInvalidateApplied` is the only accepted v1 confirmation
level; keeping it explicit prevents W from being confused with hop receipt or
value visibility. `WriteOptions` is the single shared definition used by the
public functional options, fake Hub seam and RPC request mapping.

### Partition routing

- Freeze the hash as **xxHash64** over the raw key bytes with a fixed seed
  constant `partitionSeed = 0x9E3779B185EBCA87`; partition index is
  `hash % PartitionCount`. `PartitionCount` is fixed for a hub generation.
- Golden vectors live at `internal/wire/testdata/partition_vectors.json`
  (`{key_hex, partition_count, partition_id}`) and are consumed by both sides.

```go
func PartitionOf(key []byte, partitionCount uint32) uint32
```

### Request identity and API model

- `MutationID` is a 16-byte value: 8 bytes node-epoch + 8 bytes monotonic
  counter, minted once by the Node and held stable across retries.
- Bounded limits (constants in `internal/wire`): `MaxKeyLen = 4 KiB`,
  `MaxValueLen = 1 MiB`, TTL in whole milliseconds, `TTLNone = 0`.
- Status codes are a closed `uint16` enum: `OK`, `NOT_FOUND`, `NOT_CAUGHT_UP`,
  `ERR_DURABILITY_UNAVAILABLE`, `ERR_BAD_GENERATION`, `ERR_RATE_LIMITED`,
  `ERR_INVALID_ARGUMENT`, `ERR_WRITE_CONFIRM_TIMEOUT`, `ERR_INTERNAL`.
  Retryable vs terminal classification is frozen in P3.
- Protocol compatibility: `ProtocolVersion uint16` plus a min-supported field in
  the handshake; mismatch fails closed.

### Byte ownership

Every boundary that retains or returns a caller/response slice copies it
(`append([]byte(nil), b...)`). The fake and real hub share one
`internal/wire.CopyBytes` helper to keep the rule uniform.

## Cross-component verification

- [ ] Hub fake and Node client compile against one contract.
- [ ] Both sides pass the same routing and encoding vectors.

**Exit:** one Node performs Get/Set/Delete against the fake authoritative Hub
without either side redefining shared types.

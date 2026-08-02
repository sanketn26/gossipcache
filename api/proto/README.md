# GossipCache protocol (protobuf / gRPC)

**Source of truth for the on-wire schema.** Hand-rolled binary frames are not used.

| Artifact | Location |
|----------|----------|
| `.proto` IDL | `api/proto/gossipcache/v1/` |
| Generated Go | `api/gen/gossipcache/v1/` (do not edit) |
| Domain validation + conversion | `internal/wire`, `internal/rpc`, `internal/control` |

## Services

| Service | RPCs | Role |
|---------|------|------|
| `Data` | `Handshake`, `HubStatus`, `Get`, `Mutate` | Authoritative L1 ↔ L2 value plane |
| `Control` | `Connect` (bidi stream) | Hub → node invalidations; apply acks; W confirms; replay |

Both services share **one** mTLS gRPC listener (`internal/rpc.DefaultRPCPort` = 7400).
v1 does not support a split control port.

Control messages are capped at `MaxControlMessageBytes` (3 MiB). Every Data
`Get`/`Mutate` carries `hub_generation` so generation mismatches fail before
commit.

## Regenerate stubs

Requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` on `PATH`:

```bash
make install-proto-tools   # plugins only; install protoc via OS package
make proto
```

Or: `./scripts/generate-proto.sh`

## Design notes

- Values never ride the control stream (SEMANTICS).
- Application apply acks are explicit (`StreamAcknowledgement`); transport receipt is gRPC/HTTP2 flow control (no hop-ack message).
- Application-level `Status` enums live in response messages; gRPC status is for transport/auth failures.
- `internal/wire` remains the Go domain model for L1; convert at the gRPC boundary via `internal/rpc` and `internal/control`.

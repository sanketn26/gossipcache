# GossipCache

In-process **L1** cache + native memory-first **L2 hub**. Hot reads stay local;
the Hub owns runtime versions and invalidations. Restart durability is opt-in.

**Caches must be local.** If every read needs a network hop, you have not solved caching.

## v1 target

- Embedded L1 (hits stay in-process)
- Memory-first L2 Hub as runtime authority (value + version + invalidation on every write)
- Optional synchronous durability profile for restart recovery
- Per-write `WriteFast` (memory acknowledgement) or `WriteSync` (durability fence)
- Control plane: mTLS **gRPC** invalidations (key + version); values via L2 Data RPC on miss
- Wire schema: protobuf (`api/proto`); no custom binary frames
- Partitioned streams; interest + held-key apply
- Tunable write **W** (default 0 = async peers)
- Stale-serve policies; consistency-aware readiness

**Not v1:** independent full-value gossip, Redis-as-SoT.

## Status

**Common contracts partial P0–P3:** domain types, partition routing, gRPC/protobuf
schema, control/data conversion helpers. The Hub runtime, Node facade, stream
origin/consumer, and multi-node demos are not implemented yet.

Honest inventory: **[docs/STATUS.md](docs/STATUS.md)**  
Locked semantics: **[docs/SEMANTICS.md](docs/SEMANTICS.md)**

Build phases: **[common contracts · hub · node](docs/impl/README.md)**

## Documentation

| Doc | Role |
|-----|------|
| **[docs/SEMANTICS.md](docs/SEMANTICS.md)** | **All semantics and choices** |
| [docs/STATUS.md](docs/STATUS.md) | What is actually built |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Short overview |
| [docs/impl/README.md](docs/impl/README.md) | Common, Hub and Node files per phase |
| [docs/README.md](docs/README.md) | Full index |

## Develop

```bash
make test        # go test -v -race ./...
make test-short
make fmt
make vet
make proto       # regenerate gRPC stubs (requires protoc + plugins)
```

## License

See [LICENSE](LICENSE).

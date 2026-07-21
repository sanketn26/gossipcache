# GossipCache

In-process **L1** cache + native authoritative **L2 hub**. Hot reads stay local; the hub owns versions and invalidations.

**Caches must be local.** If every read needs a network hop, you have not solved caching.

## v1 features

- Embedded L1 (hits in microseconds–milliseconds range depending on hardware)
- L2 hub as source of truth (durable version + invalidation on every write)
- Control plane: mTLS TCP invalidations (key + version); values via L2 RPC on miss
- Partitioned streams for hub scale; interest + held-key apply (not update-all-nodes)
- Tunable write **W** (default 0 = async peers; higher W optional and costly)
- Stale-serve policies; consistency-aware readiness

**Not v1 focus:** independent full-value gossip, Redis-as-SoT.

## Status

Early design / pre-implementation. Semantics are locked in docs; code follows.

## Documentation

| Doc | Role |
|-----|------|
| **[docs/SEMANTICS.md](docs/SEMANTICS.md)** | **All semantics and choices** |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Short overview |
| [docs/diagrams/SEQUENCES.md](docs/diagrams/SEQUENCES.md) | Flows |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Deploy sketch |
| [docs/impl/TESTING_STRATEGY.md](docs/impl/TESTING_STRATEGY.md) | Test plan |
| [docs/README.md](docs/README.md) | Full index |

## Develop

```bash
go test ./...
go test -cover ./...
go build ./...
```

## License

See [LICENSE](LICENSE).

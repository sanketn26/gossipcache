# GossipCache

In-process **L1** cache + native authoritative **L2 hub**. Hot reads stay local; the hub owns versions and invalidations.

**Caches must be local.** If every read needs a network hop, you have not solved caching.

## v1 target

- Embedded L1 (hits stay in-process)
- L2 hub as source of truth (durable version + invalidation on every write)
- Control plane: mTLS TCP invalidations (key + version); values via L2 RPC on miss
- Partitioned streams; interest + held-key apply
- Tunable write **W** (default 0 = async peers)
- Stale-serve policies; consistency-aware readiness

**Not v1:** independent full-value gossip, Redis-as-SoT.

## Status

**Local L1 foundation only** (in-memory cache, config, basic metrics). Hybrid hub, streams, and multi-node demos are not implemented yet.

Honest inventory: **[docs/STATUS.md](docs/STATUS.md)**  
Locked semantics: **[docs/SEMANTICS.md](docs/SEMANTICS.md)**  
Build plan: **[docs/impl/PHASE_PLAN.md](docs/impl/PHASE_PLAN.md)** (P0–P8)

## Documentation

| Doc | Role |
|-----|------|
| **[docs/SEMANTICS.md](docs/SEMANTICS.md)** | **All semantics and choices** |
| [docs/STATUS.md](docs/STATUS.md) | What is actually built |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Short overview |
| [docs/impl/PHASE_PLAN.md](docs/impl/PHASE_PLAN.md) | Phases P0–P8 |
| [docs/README.md](docs/README.md) | Full index |

## Develop

```bash
make test        # go test -v -race ./...
make test-short
make fmt
make vet
make build       # example binary (library has no default binary)
```

Library use today (local memory only):

```go
import "github.com/sanketn26/gossipcache/pkg/gossipcache/inmemory"

cache, err := inmemory.New(inmemory.Options{
    MaxSize:    1 << 30,
    DefaultTTL: 5 * time.Minute,
})
```

## License

See [LICENSE](LICENSE).

# GossipCache

A distributed in-memory cache that automatically synchronizes across nodes using the gossip protocol.

## Overview

GossipCache is a high-performance, distributed key-value cache designed to provide microsecond-level data access while maintaining eventual consistency across a cluster of nodes.

**Core Philosophy**: Caches must be local. If accessing a cache requires a network call, you're just pushing the problem elsewhere.

## Features

- **Two Operating Modes**:
  - **Backed Mode**: Uses Redis/Valkey/Postgres as backing store with metadata-only gossip
  - **Independent Mode**: Pure distributed cache with no external dependencies

- **Microsecond-Level Access**: Local in-memory cache eliminates network latency
- **Efficient Gossip Protocol**: Metadata-only gossip in backed mode uses 20x less bandwidth
- **Flexible Deployment**: Supports EC2, Docker, and Kubernetes
- **Automatic Synchronization**: Gossip protocol maintains eventual consistency
- **Conflict Resolution**: Vector clocks for independent mode, backing store as truth for backed mode
- **Graceful Degradation**: Serve stale data when backing store is unavailable

## Quick Start

GossipCache is primarily a Go library. Import it and construct an in-memory
cache through the public constructor:

```bash
go get github.com/sanketn26/gossipcache
```

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/sanketn26/gossipcache/pkg/gossipcache"
    "github.com/sanketn26/gossipcache/pkg/gossipcache/inmemory"
)

func main() {
    cache, err := inmemory.New(inmemory.Options{
        MaxSize:    1 << 30,           // 1 GiB soft cap
        DefaultTTL: 5 * time.Minute,
    })
    if err != nil {
        panic(err)
    }
    defer cache.Close()

    ctx := context.Background()
    if err := cache.Set(ctx, "hello", []byte("world"), 0); err != nil {
        panic(err)
    }

    value, err := cache.Get(ctx, "hello")
    if err != nil {
        panic(err)
    }
    fmt.Println(string(value))

    // Public sentinels work with errors.Is across the package boundary.
    if _, err := cache.Get(ctx, "missing"); err != nil {
        fmt.Println(err, "==", gossipcache.ErrKeyNotFound)
    }
}
```

### Optional example binary

A runnable example with metrics, config loading, and graceful shutdown lives
under [`examples/server`](examples/server). It is excluded from the default
build via the `example` build tag so library consumers do not pull in CLI
dependencies:

```bash
# Build the example binary
go build -tags example -o bin/gossipcache-example ./examples/server

# Or run directly
go run -tags example ./examples/server -config config.yaml
```

The Makefile targets `make build`, `make run`, and `make install` all build
this example.

## Documentation

Comprehensive documentation is available in the [docs/](docs/) directory:

- **[Architecture Overview](docs/ARCHITECTURE.md)** - System design, components, and trade-offs
- **[Technical Specification](docs/TECHNICAL_SPEC.md)** - Detailed specs, data structures, and APIs
- **[Deployment Guide](docs/DEPLOYMENT.md)** - Deploy on EC2, Docker, or Kubernetes
- **[Demo and Polish Plan](docs/impl/PHASE_5_DEMO_POLISH_SPONSORSHIP.md)** - Post-Phase 4 demo, repo polish, and Buy Me a Coffee setup
- **[Sequence Diagrams](docs/diagrams/)** - Visual flows for both operating modes

See [docs/README.md](docs/README.md) for full documentation index.

## Performance

| Operation | Target Latency | Comparison |
|-----------|----------------|------------|
| Cache Hit | < 1ms | 100-1000x faster than DB |
| Redis | 1-5ms | 5-10x slower than local |
| Database | 10-100ms+ | Network + query overhead |

## Use Cases

### Backed Mode
- High-read, low-write workloads
- Product catalogs, configuration data
- Reducing database/Redis load
- Applications tolerating slight staleness

### Independent Mode
- Service discovery/registry
- Distributed rate limiting
- Session storage
- Feature flags and configuration

## Project Status

⚠️ **Early Development** — The local in-memory cache (sharded storage, LRU eviction, TTL, metrics, config) and a Redis backing-store adapter are implemented and tested. The distributed layer (gossip, membership, anti-entropy) is not built yet, so the features above describe the target design, not current capability.

See [docs/STATUS.md](docs/STATUS.md) for the authoritative breakdown of what is implemented, in progress, and out of scope for v1.

## Contributing

*Contribution guidelines coming soon*

## License

See [LICENSE](LICENSE) file for details.

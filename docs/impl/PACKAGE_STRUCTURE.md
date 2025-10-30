# Go Package Structure

Complete package organization for GossipCache following Go best practices and SOLID principles.

## Directory Layout

```
gossipcache/
├── cmd/
│   └── gossipcache/
│       └── main.go                    # Application entry point
├── internal/                          # Private application code
│   ├── cache/
│   │   ├── cache.go                  # Cache manager interface
│   │   ├── backed_cache.go           # Backed mode implementation
│   │   ├── independent_cache.go      # Independent mode implementation
│   │   ├── entry.go                  # Cache entry types
│   │   └── stats.go                  # Statistics
│   ├── storage/
│   │   ├── storage.go                # Storage interface
│   │   ├── entry.go                  # Storage entry types
│   │   ├── memory/
│   │   │   ├── memory.go             # In-memory implementation
│   │   │   ├── sharded.go            # Sharded map
│   │   │   ├── eviction.go           # Eviction policy interface
│   │   │   ├── lru.go                # LRU policy
│   │   │   └── lfu.go                # LFU policy
│   │   └── mock/
│   │       └── mock_storage.go       # Mock for testing
│   ├── backingstore/
│   │   ├── backingstore.go           # Backing store interface
│   │   ├── config.go                 # Backing store config
│   │   ├── redis/
│   │   │   ├── redis.go              # Redis implementation
│   │   │   └── redis_test.go
│   │   ├── postgres/
│   │   │   ├── postgres.go           # Postgres implementation
│   │   │   └── postgres_test.go
│   │   ├── mysql/
│   │   │   └── mysql.go              # MySQL implementation
│   │   └── mock/
│   │       └── mock_backingstore.go
│   ├── gossip/
│   │   ├── engine.go                 # Base gossip engine
│   │   ├── backed_engine.go          # Backed mode gossip
│   │   ├── independent_engine.go     # Independent mode gossip
│   │   ├── message.go                # Message types
│   │   ├── protocol.go               # Protocol implementation
│   │   ├── peer.go                   # Peer management
│   │   ├── antientropy.go            # Anti-entropy
│   │   └── merkle.go                 # Merkle tree
│   ├── network/
│   │   ├── transport.go              # TCP/UDP transport
│   │   ├── codec.go                  # Message encoding/decoding
│   │   ├── discovery.go              # Discovery interface
│   │   ├── static_discovery.go       # Static peers
│   │   ├── ec2_discovery.go          # EC2 discovery
│   │   ├── docker_discovery.go       # Docker discovery
│   │   └── k8s_discovery.go          # Kubernetes discovery
│   ├── vclock/
│   │   ├── vclock.go                 # Vector clock
│   │   ├── comparator.go             # Clock comparison
│   │   └── vclock_test.go
│   ├── conflict/
│   │   ├── resolver.go               # Resolver interface
│   │   ├── lww.go                    # Last-write-wins
│   │   ├── custom.go                 # Custom merge
│   │   └── siblings.go               # Keep siblings
│   ├── config/
│   │   ├── config.go                 # Config structs
│   │   ├── loader.go                 # Load from file/env
│   │   └── validator.go              # Validation
│   ├── observability/
│   │   ├── logger.go                 # Structured logging
│   │   ├── metrics.go                # Prometheus metrics
│   │   └── tracing.go                # OpenTelemetry (optional)
│   ├── api/
│   │   ├── server.go                 # HTTP server
│   │   ├── handlers.go               # HTTP handlers
│   │   └── middleware.go             # HTTP middleware
│   └── util/
│       ├── singleflight.go           # Singleflight pattern
│       ├── hash.go                   # Hashing utilities
│       └── time.go                   # Time utilities
├── pkg/
│   └── gossipcache/
│       ├── client.go                 # Public client API
│       ├── cache.go                  # Cache interface
│       ├── types.go                  # Public types
│       └── errors.go                 # Public errors
├── test/
│   ├── integration/
│   │   ├── backed_mode_test.go
│   │   ├── independent_mode_test.go
│   │   └── multi_node_test.go
│   ├── benchmark/
│   │   └── cache_bench_test.go
│   ├── chaos/
│   │   ├── partition_test.go
│   │   └── failures_test.go
│   ├── load/
│   │   └── load_test.go
│   └── helpers/
│       ├── cluster.go                # Test cluster setup
│       ├── network.go                # Network simulation
│       └── fixtures.go               # Test data
├── deployments/
│   ├── docker/
│   │   ├── Dockerfile
│   │   └── docker-compose.yml
│   ├── kubernetes/
│   │   ├── namespace.yaml
│   │   ├── configmap.yaml
│   │   ├── statefulset.yaml
│   │   ├── service.yaml
│   │   └── rbac.yaml
│   └── terraform/
│       └── # Infrastructure as code
├── scripts/
│   ├── build.sh                      # Build script
│   ├── test.sh                       # Test script
│   └── deploy.sh                     # Deployment script
├── configs/
│   ├── config.yaml                   # Default config
│   ├── backed.yaml                   # Backed mode config
│   └── independent.yaml              # Independent mode config
├── docs/                             # Documentation
├── .github/
│   └── workflows/
│       ├── test.yml                  # CI tests
│       └── release.yml               # Release workflow
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── LICENSE
└── .gitignore
```

## Package Responsibilities

### `cmd/gossipcache`
**Purpose**: Application entry point
**Responsibilities**:
- Parse command-line flags
- Load configuration
- Initialize components
- Start services
- Handle signals

### `internal/cache`
**Purpose**: Cache coordination layer
**Responsibilities**:
- Implement Cache interface
- Coordinate between storage, gossip, and backing store
- Handle cache operations (Get, Set, Delete)
- Track statistics

### `internal/storage`
**Purpose**: Local data storage
**Responsibilities**:
- In-memory storage implementation
- TTL and expiration handling
- Eviction policies
- Concurrency control

### `internal/backingstore`
**Purpose**: External persistent storage
**Responsibilities**:
- Backing store abstraction
- Redis, Postgres, MySQL implementations
- Connection pooling
- Version management

### `internal/gossip`
**Purpose**: Gossip protocol implementation
**Responsibilities**:
- Metadata gossip (backed mode)
- Full-data gossip (independent mode)
- Peer management
- Anti-entropy

### `internal/network`
**Purpose**: Network communication
**Responsibilities**:
- TCP/UDP transport
- Message encoding/decoding
- Node discovery
- Connection management

### `internal/vclock`
**Purpose**: Vector clock implementation
**Responsibilities**:
- Vector clock operations
- Clock comparison
- Causality tracking

### `internal/conflict`
**Purpose**: Conflict resolution
**Responsibilities**:
- Resolution strategies
- LWW, custom merge, siblings
- Conflict detection

### `internal/config`
**Purpose**: Configuration management
**Responsibilities**:
- Load from file/environment
- Validation
- Default values

### `internal/observability`
**Purpose**: Logging and monitoring
**Responsibilities**:
- Structured logging
- Prometheus metrics
- Health checks

### `internal/api`
**Purpose**: HTTP API
**Responsibilities**:
- REST endpoints
- Request handling
- Middleware

### `pkg/gossipcache`
**Purpose**: Public API
**Responsibilities**:
- Client interface
- Public types
- Error definitions

## Import Rules

### Internal Packages
- Can import from other `internal/` packages
- Cannot be imported outside the project

### Public Packages (`pkg/`)
- Should have minimal dependencies
- Can be imported by external projects
- Only expose necessary types

### Dependency Direction
```
cmd/gossipcache
    ↓
pkg/gossipcache (public API)
    ↓
internal/cache (coordination)
    ↓
internal/{storage, gossip, backingstore} (implementations)
    ↓
internal/{network, vclock, config, util} (utilities)
```

## Naming Conventions

### Files
- `thing.go` - Main implementation
- `thing_test.go` - Unit tests
- `thing_internal_test.go` - Internal tests (same package)
- `mock_thing.go` - Mocks

### Interfaces
- Use clear, descriptive names: `Storage`, `BackingStore`, `Resolver`
- Prefer `-er` suffix for single-method interfaces: `Resolver`, `Comparator`

### Constructors
- `New()` for package-level constructor
- `NewThing()` for specific type

### Methods
- Use short receiver names: `(c *Cache)`, `(s *Storage)`
- Be consistent within a package

## Testing Package Structure

```
# Unit tests alongside code
internal/storage/memory/
├── memory.go
└── memory_test.go

# Integration tests separate
test/integration/
├── backed_mode_test.go
└── independent_mode_test.go

# Test helpers
test/helpers/
├── cluster.go          # DRY: Reusable cluster setup
└── network.go          # DRY: Network simulation
```

## Example Import Paths

```go
import (
    "github.com/yourorg/gossipcache/pkg/gossipcache"
    "github.com/yourorg/gossipcache/internal/cache"
    "github.com/yourorg/gossipcache/internal/storage/memory"
    "github.com/yourorg/gossipcache/internal/backingstore/redis"
)
```

## Initialization Example

```go
// cmd/gossipcache/main.go
package main

func main() {
    // Load config
    cfg := config.Load("config.yaml")

    // Setup logger
    logger := observability.NewLogger(cfg.Logging)

    // Create storage
    storage := memory.New(cfg.Cache.MaxSize, cfg.Cache.EvictionPolicy)

    // Create backing store (if backed mode)
    var backingStore backingstore.BackingStore
    if cfg.Mode == config.ModeBacked {
        backingStore = redis.New(cfg.BackingStore)
    }

    // Create transport
    transport := network.NewTransport(cfg.Network.TCPPort, cfg.Network.UDPPort)

    // Create gossip engine
    gossipEngine := gossip.NewEngine(cfg.NodeID, transport, storage, backingStore, cfg.Gossip)

    // Create cache
    cache := cache.NewBackedCache(storage, backingStore, gossipEngine, cfg.Cache)

    // Start services
    cache.Start()
    gossipEngine.Start()

    // Graceful shutdown
    <-waitForSignal()
    cache.Stop()
}
```

## Best Practices

1. **Keep packages focused**: Each package should have a single, clear responsibility
2. **Minimize dependencies**: Avoid circular dependencies
3. **Use interfaces**: Define interfaces where consumers use them
4. **Export minimally**: Only export what's necessary
5. **Group by feature**: Not by layer (avoid `models/`, `controllers/` packages)
6. **Test in same package**: Use `_test.go` suffix, same package name

## References

- [Go Project Layout](https://github.com/golang-standards/project-layout)
- [Go Package Best Practices](https://rakyll.org/style-packages/)
- [Organizing Go Code](https://go.dev/blog/organizing-go-code)

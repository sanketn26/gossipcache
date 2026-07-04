# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GossipCache is a distributed key-value cache that automatically synchronizes across nodes using the gossip protocol. This allows for eventual consistency across a cluster of cache nodes without requiring a central coordinator.

**Core Philosophy**: Caches must be local. If accessing a cache requires a network call, you're just pushing the problem elsewhere. GossipCache provides in-memory, local cache with microsecond access times while using gossip protocol to maintain consistency across nodes.

## Project Status

This is an early-stage project. The codebase structure and commands below will be populated as the project develops.

## Development Commands

```bash
# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...

# Run a specific test
go test -run TestName ./path/to/package

# Build the project
go build ./...

# Format code
go fmt ./...

# Run linter (requires golangci-lint)
golangci-lint run

# Tidy dependencies
go mod tidy
```

## Architecture

**Operating Modes:**

GossipCache supports two distinct operating modes:

1. **Backed Mode**:
   - Primary data source is a backing store (Redis/Valkey, Postgres, or other databases)
   - Cache nodes auto-synchronize with the backing store
   - Acts as a distributed read-through/write-through cache layer
   - Backing store serves as source of truth
   - **Development Roadmap**: Starting with Redis/Valkey support, then Postgres, then generic DB abstraction

2. **Independent Mode**:
   - True distributed cache with no external backing store
   - Any node can accept writes
   - Updates automatically propagate across nodes via gossip
   - Eventual consistency across all nodes

**Deployment Targets:**
- EC2 instances (direct node-to-node communication)
- Docker containers (network discovery and communication)
- Kubernetes pods (service discovery via K8s API or DNS)

**Core Components:**

- **Gossip Protocol**: Hybrid approach optimized per mode
  - **Backed Mode**: Gossip propagates metadata only (key, version, checksum) - minimal bandwidth
    - On version mismatch → Pull actual data from backing store
    - Gossip acts as change detection/invalidation mechanism
    - Scales efficiently: gossip overhead constant regardless of value size
  - **Independent Mode**: Gossip propagates full data (key + value + vector clock)
    - No backing store available, gossip must carry actual values
    - Direct propagation for faster consistency
  - Both modes use anti-entropy for catching missed updates

- **Cache Storage Engine**: In-memory map with concurrency control (sync.Map or sharded locks) - local access in microseconds

- **Mode Abstraction**: Interface to support both backed and independent modes

- **Backing Store Abstraction**: Generic interface to support multiple backing stores
  - Initial implementation: Redis/Valkey client
  - Planned: Postgres connector (using LISTEN/NOTIFY or polling)
  - Future: MySQL, MongoDB, DynamoDB, etc.
  - **Pull Strategy**: Nodes pull data from backing store when gossip indicates changes
  - **Singleflight Pattern**: Prevent thundering herd when multiple nodes pull same key

- **Node Discovery**: Environment-specific discovery mechanisms (EC2 metadata, Docker networking, K8s API)

- **Network Layer**: Node-to-node communication (TCP/UDP)

- **Membership Management**: Tracking active nodes in the cluster

- **Conflict Resolution**:
  - Backed Mode: Backing store is source of truth, version numbers for ordering
  - Independent Mode: Vector clocks for conflict detection, configurable resolution (last-write-wins, custom merge)

- **Anti-Entropy**: Periodic full-state reconciliation to ensure consistency

**Performance Goals:**
- Local cache access: < 1ms (in-memory speed)
- Compare to: Redis over network (1-5ms), Database queries (10-100ms+)
- Target: 100-1000x faster than direct database access
- Gossip bandwidth efficiency: Metadata-only gossip in backed mode uses ~20x less bandwidth than full-data gossip

**Design Decisions:**

1. **Gossip Optimization**: Use gossip for different purposes per mode
   - Backed mode: Gossip = change detection, Pull = data fetch (bandwidth efficient)
   - Independent mode: Gossip = data propagation (no choice, must carry data)

2. **Consistency Model**: Eventual consistency via gossip + anti-entropy
   - Acceptable staleness window (configurable via gossip interval and TTL)
   - Trade-off: Lower latency and higher availability vs strict consistency

3. **Failure Modes**:
   - Backed mode: Serve stale cache if backing store unavailable
   - Independent mode: Partition tolerance via vector clocks
   - Both: TTL-based expiration ensures bounded staleness

## Coding Guidelines

- Follow existing Go style and package boundaries.
- Keep public API in `pkg/gossipcache` small and stable.
- Put private implementation details under `internal/`.
- Preserve context-aware APIs. Existing cache and storage calls accept `context.Context`; new I/O, cache, gossip, or storage operations should do the same.
- Copy mutable byte slices at API/storage boundaries when retaining or returning values, so callers cannot mutate internal cache state.
- Keep concurrency changes race-safe. The default full test target uses `-race`; use it for changes touching shared state, goroutines, storage, or metrics.
- Avoid introducing network calls into local cache read paths unless the mode semantics explicitly require them.
- Prefer interfaces at package boundaries where the repo already uses them; avoid broad abstractions without a concrete caller.
- Keep comments useful and sparse. Document exported identifiers and non-obvious concurrency or consistency behavior.

## Go Software Development Best Practices

- Write simple, readable Go before clever Go.
- Use clear package boundaries. CLI entry points, configuration, cache orchestration, storage engines, observability, network/gossip behavior, and public client APIs should remain separate.
- Prefer small functions with explicit inputs, outputs, and error returns.
- Keep interfaces small and consumer-owned where practical. Define an interface close to the package that uses it, not automatically next to the implementation.
- Accept `context.Context` as the first argument for operations that may block, perform I/O, coordinate goroutines, or depend on cancellation.
- Return errors instead of panicking. Panics should be reserved for programmer errors or truly unrecoverable initialization mistakes.
- Wrap errors with useful context using `fmt.Errorf("...: %w", err)` and preserve sentinel errors where callers rely on `errors.Is`.
- Avoid package-level mutable state except for well-defined constants, registries, or metrics that are safe for concurrent use.
- Keep side effects at the edges. File I/O, subprocess execution, network calls, backing-store calls, and peer communication should be isolated behind small APIs that tests can replace.
- Validate all external inputs, especially config files, environment variables, node addresses, cache keys, TTLs, max sizes, protocol messages, and backing-store responses.
- Fail safely with actionable error messages.
- Do not ignore errors. If an error is intentionally ignored, make the reason obvious in code.
- Prefer dependency injection for storage engines, backing stores, clocks/timers when needed, loggers, metrics collectors, transports, and discovery providers.
- Copy mutable `[]byte`, maps, and slices when crossing API or storage boundaries if later mutation would corrupt internal state.
- Keep goroutine lifetimes explicit. Every background goroutine should have a cancellation or close path, and shutdown should be idempotent.
- Use `sync.Mutex`, `sync.RWMutex`, `atomic`, and channels deliberately. Favor the simplest concurrency primitive that fits the ownership model.
- Keep tests deterministic. Avoid sleeps when a controllable clock, short interval, or direct method call can make behavior reliable.
- Use `t.TempDir()` for filesystem behavior in tests.
- Avoid real network, backing-store, Docker, or service calls in unit tests. Keep integration and long-running tests separate from the normal package suite.
- Add tests for every behavior change, especially around TTLs, eviction, config validation, concurrent access, close semantics, gossip protocol behavior, and public API errors.
- Maintain backwards compatibility for exported types, methods, config keys, metrics names, and documented behavior unless intentionally changing them.
- Avoid broad refactors while fixing narrow issues.
- Make logging useful for debugging distributed behavior, but avoid leaking cache values, credentials, connection strings, tokens, or sensitive config.
- Prefer explicit configuration through config files, environment variables, or typed defaults in `internal/config` rather than hidden behavior.
- When adding new storage engines, backing stores, discovery providers, gossip message types, config options, metrics, or public API methods, update the implementation, docs, and tests together.

## SOLID Principles For Go

### Single Responsibility Principle (SRP)

- Each package, type, and function should have one clear reason to change.
- Keep command startup, config loading, cache coordination, storage, gossip/network behavior, observability, and public API wrappers separated.
- Avoid large manager types that own configuration, storage, networking, metrics, retries, and protocol behavior all at once.
- Cache orchestration should coordinate dependencies, not embed storage-engine or transport-specific details directly.

### Open/Closed Principle (OCP)

- Design stable behavior so it can be extended without repeatedly editing core orchestration.
- New storage engines, backing stores, eviction policies, discovery mechanisms, conflict resolvers, and transports should fit behind small interfaces or registration points when there is more than one real implementation.
- Prefer adapters and strategy types over large conditional chains when behavior is expected to grow.
- Do not add abstraction only for imagined future needs; add it when there is a concrete extension point in this repo.

### Liskov Substitution Principle (LSP)

- Implementations of an interface should be safely interchangeable.
- Any `storage.Storage` implementation should preserve expected return values, copy semantics, context behavior, close behavior, and error semantics.
- Mock or fake implementations in tests should behave like production implementations where callers depend on behavior.
- Avoid leaking implementation-specific assumptions from memory storage, backing stores, or transports into shared cache orchestration.

### Interface Segregation Principle (ISP)

- Keep interfaces focused and minimal.
- Do not force storage engines, backing stores, discovery providers, or transports to implement methods they do not need.
- Split read-only, mutating, lifecycle, streaming, or batch capabilities when a caller only needs one subset.
- Prefer small Go interfaces over large abstract contracts.

### Dependency Inversion Principle (DIP)

- High-level cache and service orchestration should depend on abstractions, not concrete storage, backing-store, discovery, or transport implementations.
- Inject dependencies instead of constructing concrete implementations deep inside orchestration code.
- Keep SDKs, wire formats, external clients, and provider-specific behavior isolated behind adapter packages.
- Make components replaceable for tests, local development, alternative backing stores, sandboxed execution, and future deployment environments.

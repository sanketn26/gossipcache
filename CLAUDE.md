# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GossipCache is a distributed key-value cache that automatically synchronizes across nodes using the gossip protocol. This allows for eventual consistency across a cluster of cache nodes without requiring a central coordinator.

**Core Philosophy**: Caches must be local. If accessing a cache requires a network call, you're just pushing the problem elsewhere. GossipCache provides in-memory, local cache with microsecond access times while using gossip protocol to maintain consistency across nodes.

## Project Status

This is an early-stage project. The codebase structure and commands below will be populated as the project develops.

## Development Commands

```bash
# Initialize Go module (if not already done)
go mod init github.com/sanketnaik/gossipcache

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

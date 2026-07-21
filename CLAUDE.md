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

**Locked semantics:** [docs/SEMANTICS.md](docs/SEMANTICS.md)
**Implementation detail:** [docs/impl/HYBRID_BACKED_MODE.md](docs/impl/HYBRID_BACKED_MODE.md)

**Operating Modes:**

1. **Backed Mode (canonical hybrid)**:
   - Embedded **L1** library in each application process
   - Native authoritative **L2** hub (partitioned, durable journal, HA in production)
   - Miss/write: point-to-point L2 RPC; singleflight on concurrent misses
   - Invalidations: L2 changefeed is the **sole publisher**; L1 peers fan out over mTLS TCP with stream watermarks, replay, gap detection, anti-entropy
   - Redis/Postgres are **not** the v1 source of truth

2. **Independent Mode**:
   - No L2; any node may accept writes
   - Full key/value/vector-clock gossip
   - Eventual consistency with configurable conflict resolution

**Deployment Targets:**
- Kubernetes (primary production profile for readiness)
- EC2, Docker
- MicroVM optional (not first-release gate)

**Core Components:**

- **L1 state machine**: EMPTY / FETCHING / VALID / STALE; stale-serve policies
- **L2 hub**: version tag `(partition_id, partition_term, sequence)`, separate `cluster_generation`, fencing, tiers, changefeed
- **Control plane**: `stream_id = (partition_id, partition_term)`; L2-only `stream_sequence`; application acks after apply; `StreamCheckpoint` freshness
- **Independent gossip**: full-value propagation + vector clocks
- **Anti-entropy**: held-key summaries vs L2 (backed); merkle/full sync (independent)
- **Observability**: readiness as consistency signal (gaps + freshness); H4 minimum metrics; Phase 5 full suite

**Performance Goals:**
- Local L1 hit: &lt; 1ms p99 objective; sub-µs only as measured benchmark claim
- Invalidation convergence: provisional p99 &lt; 500ms under hybrid reference load profile
- Control plane carries invalidations only (not values) in backed mode

**Design Decisions:**

1. L2-only invalidation publish (no dual publishers)
2. TCP + app-level delivery guarantees (no custom RUDP for v1)
3. Eventual consistency with explicit stale-serve and reconciliation-before-ready
4. Independent mode unchanged in spirit (full-value gossip)

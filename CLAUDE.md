# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GossipCache is a hybrid **L1 + native L2 hub** cache: hot reads stay in-process;
the hub owns versions and invalidation streams. See [docs/SEMANTICS.md](docs/SEMANTICS.md).

**Core Philosophy**: Caches must be local. If every read needs a network hop, you have not solved caching.

## Project Status

Local in-memory L1 foundation only. Hybrid hub/streams not implemented.
Honest inventory: [docs/STATUS.md](docs/STATUS.md). Phases: [docs/impl/PHASE_PLAN.md](docs/impl/PHASE_PLAN.md).

## Development Commands

```bash
make test
make test-short
make coverage
make fmt
make vet
make build
go test ./...
go test -run TestName ./path/to/package
golangci-lint run
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
- **L2 hub**: version tag `(partition_id, sequence)` + `hub_generation`; durable journal; changefeed sole invalidation publisher
- **Control plane**: mTLS TCP streams; L2-only `stream_sequence`; application acks after apply; `StreamCheckpoint` freshness
- **Anti-entropy**: held-key summaries vs L2 (hybrid)
- **Observability**: readiness as consistency signal (gaps + freshness); H4 minimum metrics; P5 full suite

**Performance Goals:**
- Local L1 hit: &lt; 1ms p99 objective; sub-µs only as measured benchmark claim
- Invalidation convergence: provisional p99 &lt; 500ms under hybrid reference load profile
- Control plane carries invalidations only (not values) in backed mode

**Design Decisions:**

1. L2-only invalidation publish (no dual publishers)
2. TCP + app-level delivery guarantees (no custom RUDP for v1)
3. Eventual consistency with explicit stale-serve and reconciliation-before-ready
4. Independent mode unchanged in spirit (full-value gossip)

# GossipCache Architecture

## Overview

GossipCache is a distributed, in-memory cache system that provides microsecond-level data access while maintaining eventual consistency across a cluster of nodes using the gossip protocol.

## Core Philosophy

**Caches must be local.** Network calls for cache access defeat the purpose of caching. GossipCache eliminates network latency by keeping data in local memory while using intelligent gossip mechanisms to maintain consistency.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Application Layer                        │
│                    (Multiple Service Instances)                  │
└─────────────────────────────────────────────────────────────────┘
                                 │
                                 │ Local In-Memory Access
                                 │ (< 1ms)
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                        GossipCache Layer                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ Cache Node 1 │  │ Cache Node 2 │  │ Cache Node N │          │
│  │              │  │              │  │              │          │
│  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │          │
│  │ │In-Memory │ │  │ │In-Memory │ │  │ │In-Memory │ │          │
│  │ │  Cache   │ │  │ │  Cache   │ │  │ │  Cache   │ │          │
│  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
│         │                 │                 │                    │
│         └─────────────────┼─────────────────┘                    │
│                           │                                      │
│                   Gossip Protocol                                │
│              (Metadata or Full Data)                             │
└─────────────────────────────────────────────────────────────────┘
                                 │
                                 │ Pull on Change Detection
                                 │ (Backed Mode Only)
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Backing Store (Optional)                      │
│         Redis / Valkey / Postgres / MySQL / MongoDB              │
└─────────────────────────────────────────────────────────────────┘
```

## Operating Modes

### Backed Mode

In backed mode, GossipCache acts as a distributed caching layer in front of a persistent backing store.

**Data Flow:**
1. Application reads from local cache (hit: return immediately)
2. On cache miss: Pull from backing store, populate local cache
3. On write: Update backing store, gossip metadata to peers
4. Peers detect change via gossip, pull updated data from backing store

**Benefits:**
- Dramatically reduces load on backing store
- Sub-millisecond read latency for cached data
- Backing store remains source of truth
- Graceful degradation: serve stale on backing store failure

### Independent Mode

In independent mode, GossipCache operates as a pure distributed cache with no external dependencies.

**Data Flow:**
1. Application reads from local cache
2. On write: Update local cache, gossip full data to peers
3. Peers receive data via gossip, update their local caches
4. Conflict resolution via vector clocks

**Benefits:**
- Zero external dependencies
- Simpler operational model
- Lower infrastructure costs
- Suitable for ephemeral data (sessions, feature flags)

## Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Cache Node                               │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                      API Layer                              │ │
│  │  (Get, Set, Delete, GetMulti, SetMulti)                    │ │
│  └────────────────────────────────────────────────────────────┘ │
│                             │                                     │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                   Cache Manager                             │ │
│  │  • Mode selection (Backed/Independent)                      │ │
│  │  • Request routing                                          │ │
│  │  • TTL management                                           │ │
│  └────────────────────────────────────────────────────────────┘ │
│           │                        │                             │
│           ▼                        ▼                             │
│  ┌─────────────────┐    ┌──────────────────────┐               │
│  │  Local Storage  │    │   Backing Store      │               │
│  │    Engine       │    │     Connector        │               │
│  │                 │    │                      │               │
│  │  • sync.Map or  │    │  • Redis/Valkey     │               │
│  │    Sharded Map  │    │  • Postgres         │               │
│  │  • Concurrency  │    │  • MySQL            │               │
│  │    Control      │    │  • MongoDB          │               │
│  │  • Eviction     │    │  • Singleflight     │               │
│  └─────────────────┘    └──────────────────────┘               │
│           │                        │                             │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                    Gossip Engine                            │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌─────────────────┐  │ │
│  │  │   Message    │  │   Protocol   │  │  Anti-Entropy   │  │ │
│  │  │   Handler    │  │   Manager    │  │     Engine      │  │ │
│  │  └──────────────┘  └──────────────┘  └─────────────────┘  │ │
│  └────────────────────────────────────────────────────────────┘ │
│           │                                                      │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │              Network & Discovery Layer                      │ │
│  │  • TCP/UDP communication                                    │ │
│  │  • Node discovery (EC2/Docker/K8s)                         │ │
│  │  • Membership tracking                                      │ │
│  │  • Health checking                                          │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

## Key Design Decisions

### 1. Hybrid Gossip Protocol

**Backed Mode: Metadata Gossip**
- Gossip messages contain only: key, version, checksum (~50 bytes)
- 20x less bandwidth than full-data gossip
- Nodes pull actual data from backing store when version differs
- Scales to large values without increasing gossip overhead

**Independent Mode: Full Data Gossip**
- Gossip messages contain: key, value, vector clock
- No backing store to pull from, must propagate data
- Optimized for smaller data sets
- Direct propagation reduces latency

### 2. Consistency Model

**Eventual Consistency:**
- Acceptable staleness window (configurable)
- Trade-off: Performance & availability > strict consistency
- Use cases: Read-heavy workloads where slight staleness is acceptable

**Conflict Resolution:**
- Backed Mode: Version numbers, backing store is source of truth
- Independent Mode: Vector clocks with configurable strategies (LWW, custom merge)

### 3. Failure Handling

**Backed Mode Failures:**
- Backing store down → Serve stale cache with degraded flag
- Network partition → Continue serving local cache
- Node failure → Other nodes unaffected, gossip to remaining peers

**Independent Mode Failures:**
- Network partition → Vector clocks detect and resolve conflicts on heal
- Node failure → Data on failed node lost, but replicated on other nodes
- Split-brain → Conflict resolution on partition heal

### 4. Memory Management

**Eviction Strategies:**
- LRU (Least Recently Used)
- LFU (Least Frequently Used)
- TTL-based expiration
- Size-based limits (max memory threshold)

**Memory Efficiency:**
- Configurable max cache size per node
- Background eviction process
- Metrics for cache hit/miss rates

## Performance Characteristics

### Latency

| Operation | Backed Mode | Independent Mode |
|-----------|-------------|------------------|
| Cache Hit | < 1ms | < 1ms |
| Cache Miss | Backing store latency (1-100ms) | N/A (always local) |
| Write | Backing store + gossip | Gossip only |
| Gossip Propagation | 100-500ms (eventual) | 100-500ms (eventual) |

### Throughput

- **Read throughput**: Limited by CPU/Memory, not network (100K+ ops/sec per node)
- **Write throughput**: Limited by backing store (backed mode) or gossip fanout (independent)
- **Gossip overhead**: ~1-5% of network bandwidth with metadata gossip

### Scalability

- **Horizontal scaling**: Add more cache nodes linearly
- **Cluster size**: Tested up to N nodes (to be determined)
- **Data size**: Backed mode scales to any size, Independent mode limited by node memory

## Deployment Considerations

### EC2 Instances
- Use EC2 metadata API for node discovery
- Security groups for gossip ports
- Multi-AZ deployment for availability

### Docker Containers
- Bridge or overlay networking
- Service discovery via Docker DNS
- Environment variables for configuration

### Kubernetes
- StatefulSet or Deployment
- Service for node discovery
- ConfigMap for configuration
- Headless service for peer discovery

See [DEPLOYMENT.md](DEPLOYMENT.md) for detailed deployment guides.

## Monitoring & Observability

**Key Metrics:**
- Cache hit/miss ratio
- Gossip message rate
- Data staleness (time since last update)
- Peer connectivity status
- Backing store latency (backed mode)
- Memory usage per node
- Eviction rate

**Health Checks:**
- Liveness: Node is running
- Readiness: Node has active peers and is gossiping
- Backing store connectivity (backed mode)

## Security Considerations

- **Gossip encryption**: TLS for inter-node communication
- **Authentication**: Shared secret or mTLS for node joining
- **Authorization**: Per-key access controls (future)
- **Network isolation**: Deploy in private networks
- **Backing store security**: Use connection pooling with auth

## Future Enhancements

1. **Smart routing**: Direct reads to nodes with most recent data
2. **Partial replication**: Not all nodes cache all keys (sharding)
3. **Multi-region support**: Cross-region gossip with conflict resolution
4. **Query support**: Filter/search cached data
5. **Persistence layer**: Optional disk backing for independent mode
6. **Observability dashboard**: Built-in UI for cluster health

## References

- [Implementation Status](STATUS.md)
- [Technical Specification](TECHNICAL_SPEC.md)
- [Deployment Guide](DEPLOYMENT.md)

# GossipCache Technical Specification

## 1. Introduction

### 1.1 Purpose
This document provides detailed technical specifications for GossipCache, a distributed in-memory cache system that uses gossip protocol for consistency across nodes.

### 1.2 Scope
Covers all technical aspects including data structures, protocols, APIs, configuration, and operational characteristics.

### 1.3 Version
Version: 0.1.0 (Initial specification)

## 2. System Requirements

### 2.1 Runtime Requirements
- Go 1.21 or higher
- Linux/macOS/Windows (amd64, arm64)
- Minimum 512MB RAM per node
- Network connectivity between nodes (TCP/UDP)

### 2.2 Dependencies
- Standard library only for core functionality
- Optional: Redis/Valkey client for backed mode
- Optional: Postgres driver for backed mode
- Optional: Kubernetes client-go for K8s discovery

## 3. Data Structures

### 3.1 Cache Entry (Backed Mode)

```go
type CacheEntry struct {
    Key       string
    Value     []byte
    Version   int64      // Monotonically increasing version from backing store
    Checksum  string     // SHA256 of value for quick comparison
    TTL       time.Duration
    ExpiresAt time.Time
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 3.2 Cache Entry (Independent Mode)

```go
type CacheEntry struct {
    Key        string
    Value      []byte
    VectorClock VectorClock // Logical clock for conflict detection
    TTL        time.Duration
    ExpiresAt  time.Time
    CreatedAt  time.Time
    UpdatedAt  time.Time
    Tombstone  bool        // True if this is a delete marker
}

type VectorClock map[string]int64 // NodeID -> Clock value
```

### 3.3 Gossip Messages

#### 3.3.1 Backed Mode: Change Notification

```go
type ChangeNotification struct {
    Key       string
    Version   int64
    Checksum  string
    Timestamp time.Time
    NodeID    string
}
```

#### 3.3.2 Independent Mode: Data Update

```go
type DataUpdate struct {
    Key         string
    Value       []byte
    VectorClock VectorClock
    TTL         time.Duration
    Timestamp   time.Time
    NodeID      string
    Tombstone   bool
}
```

#### 3.3.3 Anti-Entropy Messages

```go
type AntiEntropyRequest struct {
    NodeID      string
    MerkleRoot  []byte
    KeyCount    int
    RequestID   string
}

type AntiEntropyResponse struct {
    RequestID   string
    Differences []KeyDifference
}

type KeyDifference struct {
    Key         string
    Value       []byte
    Version     int64        // Backed mode
    VectorClock VectorClock  // Independent mode
}
```

#### 3.3.4 Membership Messages

```go
type JoinRequest struct {
    NodeID    string
    Address   string
    Mode      OperatingMode
    Timestamp time.Time
}

type JoinAck struct {
    ClusterID    string
    Peers        []NodeInfo
    GossipConfig GossipConfig
}

type NodeInfo struct {
    NodeID  string
    Address string
    Status  NodeStatus // Active, Suspected, Dead
    LastSeen time.Time
}
```

### 3.4 Configuration

```go
type Config struct {
    // Mode
    Mode OperatingMode // Backed or Independent

    // Node Identity
    NodeID  string
    Address string

    // Cache Settings
    MaxCacheSize int64 // Bytes
    DefaultTTL   time.Duration
    EvictionPolicy EvictionPolicy // LRU, LFU, TTL

    // Gossip Settings
    GossipInterval    time.Duration // How often to gossip
    GossipFanout      int           // Number of peers per gossip round
    AntiEntropyInterval time.Duration // Full sync interval

    // Backing Store (Backed Mode only)
    BackingStore BackingStoreConfig

    // Network
    TCPPort int
    UDPPort int

    // Discovery
    DiscoveryMode DiscoveryMode // Static, EC2, Docker, K8s, DNS
    DiscoveryConfig interface{}

    // Timeouts
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
    GossipTimeout time.Duration

    // Conflict Resolution (Independent Mode)
    ConflictStrategy ConflictStrategy // LWW, Custom, Siblings

    // Observability
    MetricsEnabled bool
    MetricsPort    int
}

type OperatingMode int
const (
    ModeBacked OperatingMode = iota
    ModeIndependent
)

type BackingStoreConfig struct {
    Type     BackingStoreType // Redis, Postgres, etc.
    Address  string
    Database string
    Username string
    Password string
    PoolSize int
}

type DiscoveryMode int
const (
    DiscoveryStatic DiscoveryMode = iota
    DiscoveryEC2
    DiscoveryDocker
    DiscoveryKubernetes
    DiscoveryDNS
)

type DNSDiscoveryConfig struct {
    ServiceName string // e.g., "gossipcache"
    Domain      string // e.g., "default.svc.cluster.local"
    Port        int
    Interval    time.Duration
}

type BackingStoreType int
const (
    BackingStoreRedis BackingStoreType = iota
    BackingStorePostgres
    BackingStoreMySQL
)
```

## 4. Core Interfaces

### 4.1 Cache Interface

```go
type Cache interface {
    // Basic Operations
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error

    // Batch Operations
    GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)
    SetMulti(ctx context.Context, entries map[string][]byte, ttl time.Duration) error

    // Management
    Flush(ctx context.Context) error
    Stats(ctx context.Context) (*CacheStats, error)

    // Lifecycle
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

type CacheStats struct {
    Hits          int64
    Misses        int64
    Evictions     int64
    Size          int64 // Bytes
    Keys          int64
    PeerCount     int
    GossipRate    float64 // Messages per second
    Staleness     time.Duration // Average age of data
}
```

### 4.2 Backing Store Interface

```go
type BackingStore interface {
    Get(ctx context.Context, key string) ([]byte, int64, error) // value, version, error
    Set(ctx context.Context, key string, value []byte) (int64, error) // version, error
    Delete(ctx context.Context, key string) error

    GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)

    // Optional: push-based invalidation
    Subscribe(ctx context.Context, callback func(key string, version int64)) error

    // Health
    Ping(ctx context.Context) error
    Close() error
}
```

### 4.3 Gossip Engine Interface

```go
type GossipEngine interface {
    // Send gossip to random peers
    Gossip(ctx context.Context, message GossipMessage) error

    // Handle incoming gossip
    OnGossip(handler GossipHandler) error

    // Anti-entropy
    SyncWithPeer(ctx context.Context, peerID string) error

    // Membership
    GetPeers() []NodeInfo
    AddPeer(node NodeInfo) error
    RemovePeer(nodeID string) error

    // Lifecycle
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

type GossipMessage interface {
    Type() MessageType
    Serialize() ([]byte, error)
}

type GossipHandler func(msg GossipMessage) error
```

### 4.4 Discovery Interface

```go
type Discovery interface {
    // Find peers in the environment
    Discover(ctx context.Context) ([]NodeInfo, error)

    // Register this node
    Register(ctx context.Context, node NodeInfo) error

    // Deregister this node
    Deregister(ctx context.Context, nodeID string) error

    // Watch for peer changes
    Watch(ctx context.Context) (<-chan []NodeInfo, error)
}
```

### 4.5 Conflict Resolver Interface (Independent Mode)

```go
type ConflictResolver interface {
    // Resolve conflict between two values
    Resolve(local, remote *CacheEntry) (*CacheEntry, error)

    // Check if two vector clocks are concurrent (conflict)
    IsConcurrent(local, remote VectorClock) bool

    // Merge two vector clocks
    MergeClocks(local, remote VectorClock) VectorClock
}
```

## 5. Protocol Specifications

### 5.1 Gossip Protocol

#### 5.1.1 Message Format

All gossip messages use the following wire format:

```
┌──────────────┬───────────┬─────────────┬────────────┐
│ Magic Number │  Version  │   Type      │  Payload   │
│   (4 bytes)  │ (2 bytes) │  (2 bytes)  │ (variable) │
└──────────────┴───────────┴─────────────┴────────────┘
```

- **Magic Number**: `0x47534350` ("GSCP" - GossipCache Protocol)
- **Version**: Protocol version (current: 1)
- **Type**: Message type code
- **Payload**: Serialized message (Protocol Buffers or MessagePack)

#### 5.1.2 Message Types

```go
type MessageType int
const (
    MsgChangeNotification MessageType = 1  // Backed mode
    MsgDataUpdate         MessageType = 2  // Independent mode
    MsgAntiEntropyReq     MessageType = 3
    MsgAntiEntropyResp    MessageType = 4
    MsgJoinRequest        MessageType = 5
    MsgJoinAck            MessageType = 6
    MsgNodeSuspect        MessageType = 7
    MsgNodeAlive          MessageType = 8
    MsgPing               MessageType = 9
    MsgAck                MessageType = 10
)
```

#### 5.1.3 Gossip Round Algorithm

```
Every GossipInterval (e.g., 1 second):
1. Select random subset of peers (fanout = 3)
2. For each peer:
   a. Collect recent changes (last gossip cycle)
   b. Create appropriate message (ChangeNotification or DataUpdate)
   c. Send async (don't wait for response)
3. Update last gossip timestamp
```

#### 5.1.4 Peer Selection Strategy

```go
func SelectGossipPeers(peers []NodeInfo, fanout int) []NodeInfo {
    // Weighted random selection:
    // - Prefer recently responsive peers (weight: 0.7)
    // - Include random peers for diversity (weight: 0.3)
    // - Exclude self

    if len(peers) <= fanout {
        return peers
    }

    selected := make([]NodeInfo, 0, fanout)

    // 70% from healthy peers
    healthyCount := int(float64(fanout) * 0.7)
    selected = append(selected, selectHealthyPeers(peers, healthyCount)...)

    // 30% random (exploration)
    randomCount := fanout - healthyCount
    selected = append(selected, selectRandomPeers(peers, randomCount)...)

    return selected
}
```

### 5.2 Anti-Entropy Protocol

#### 5.2.1 Merkle Tree Construction

```go
// Build Merkle tree of all keys for efficient comparison
func BuildMerkleTree(entries []CacheEntry) *MerkleTree {
    // Leaf nodes: hash(key + version/vclock)
    // Internal nodes: hash(left + right)
    // Root: single hash representing entire keyspace
}
```

#### 5.2.2 Anti-Entropy Algorithm

```
Every AntiEntropyInterval (e.g., 5 minutes):
1. Build Merkle tree of local cache
2. Select random peer
3. Send AntiEntropyRequest with tree root
4. Peer compares roots:
   a. If same: Done
   b. If different: Exchange subtree hashes recursively
5. Identify differing keys
6. Exchange full entries for differing keys
7. Merge/resolve conflicts
8. Update local cache
```

### 5.3 Conflict Resolution (Independent Mode)

#### 5.3.1 Vector Clock Comparison

```go
func CompareVectorClocks(local, remote VectorClock) ClockRelation {
    localGreater := false
    remoteGreater := false

    // Get all unique node IDs
    allNodes := union(local.keys(), remote.keys())

    for _, nodeID := range allNodes {
        localVal := local[nodeID]
        remoteVal := remote[nodeID]

        if localVal > remoteVal {
            localGreater = true
        } else if remoteVal > localVal {
            remoteGreater = true
        }
    }

    if localGreater && !remoteGreater {
        return LocalNewer
    } else if remoteGreater && !localGreater {
        return RemoteNewer
    } else if localGreater && remoteGreater {
        return Concurrent  // Conflict!
    } else {
        return Equal
    }
}
```

#### 5.3.2 Last-Write-Wins (LWW) Strategy

```go
func ResolveWithLWW(local, remote *CacheEntry) *CacheEntry {
    if remote.UpdatedAt.After(local.UpdatedAt) {
        // Remote is newer by wall clock
        return &CacheEntry{
            Key:         remote.Key,
            Value:       remote.Value,
            VectorClock: MergeClocks(local.VectorClock, remote.VectorClock),
            UpdatedAt:   remote.UpdatedAt,
        }
    }
    return local
}

func MergeClocks(local, remote VectorClock) VectorClock {
    merged := make(VectorClock)
    allNodes := union(local.keys(), remote.keys())

    for _, nodeID := range allNodes {
        merged[nodeID] = max(local[nodeID], remote[nodeID])
    }

    return merged
}
```

## 6. API Specification

### 6.1 HTTP API (Optional Management Interface)

```
GET    /api/v1/cache/:key              - Get value
PUT    /api/v1/cache/:key              - Set value
DELETE /api/v1/cache/:key              - Delete value
POST   /api/v1/cache/multi             - Batch get/set

GET    /api/v1/stats                   - Cache statistics
GET    /api/v1/peers                   - Cluster membership
GET    /api/v1/health                  - Health check

POST   /api/v1/admin/flush             - Flush cache
POST   /api/v1/admin/sync              - Trigger anti-entropy
```

### 6.2 Go Client Library

```go
package gossipcache

// Create new cache client
func New(config *Config) (*Client, error)

// Client methods
type Client struct {
    // Public methods match Cache interface
}

// Example usage:
client, err := gossipcache.New(&gossipcache.Config{
    Mode: gossipcache.ModeBacked,
    BackingStore: gossipcache.BackingStoreConfig{
        Type: gossipcache.BackingStoreRedis,
        Address: "localhost:6379",
    },
    GossipInterval: 1 * time.Second,
})

value, err := client.Get(ctx, "mykey")
err = client.Set(ctx, "mykey", []byte("myvalue"), 5*time.Minute)
```

## 7. Performance Targets

### 7.1 Latency

| Operation | Target | Max |
|-----------|--------|-----|
| Local Get (hit) | < 100μs | < 1ms |
| Local Set | < 200μs | < 2ms |
| Gossip Propagation | < 500ms | < 2s |
| Anti-Entropy Sync | < 5s | < 30s |

### 7.2 Throughput

| Metric | Target |
|--------|--------|
| Reads per node | > 100K ops/sec |
| Writes per node | > 50K ops/sec |
| Gossip messages | < 1K msg/sec |

### 7.3 Scalability

| Metric | Target |
|--------|--------|
| Cluster size | Up to 100 nodes |
| Cache size per node | Up to 10GB |
| Key size | Up to 1KB |
| Value size | Up to 10MB (backed), 1MB (independent) |

## 8. Failure Modes & Recovery

### 8.1 Backing Store Failure (Backed Mode)

**Scenario**: Redis/Postgres becomes unavailable

**Behavior**:
1. Continue serving from local cache (stale reads)
2. Mark node as "degraded"
3. Return stale data with `X-Cache-Stale: true` header/flag
4. Block writes (return error)
5. Retry backing store with exponential backoff
6. Resume normal operation when backing store recovers

### 8.2 Network Partition

**Scenario**: Cluster splits into multiple partitions

**Behavior** (Independent Mode):
1. Each partition continues operating independently
2. Vector clocks track divergent state
3. On partition heal, gossip resumes
4. Conflicts detected via vector clocks
5. Resolve using configured strategy
6. Converge to consistent state

**Behavior** (Backed Mode):
1. Each partition continues operating
2. All partitions can read from backing store (if reachable)
3. Writes go to backing store
4. Gossip resumes on heal
5. Pull latest versions from backing store
6. Backing store provides consistency

### 8.3 Node Failure

**Detection**: Missed heartbeats (3 consecutive gossip cycles)

**Behavior**:
1. Mark node as "suspected"
2. Continue gossip to other nodes
3. After timeout (30s), mark as "dead"
4. Remove from peer list
5. No data loss (replicated on other nodes)

**Recovery**:
1. Node rejoins via JoinRequest
2. Runs anti-entropy sync
3. Catches up on missed updates
4. Resumes normal operation

## 9. Security

### 9.1 Transport Security

- TLS 1.3 for all inter-node communication
- mTLS for mutual authentication
- Certificate rotation support

### 9.2 Authentication

```go
type AuthConfig struct {
    Enabled bool
    Method  AuthMethod // SharedSecret, mTLS, JWT
    Secret  string
}
```

### 9.3 Authorization (Future)

- Per-key ACLs
- Role-based access control
- Namespace isolation

### 9.4 Implementation Plan

Security hardening is captured as optional [Phase 4.5](impl/PHASE_4_5_SECURITY.md). It includes TLS/mTLS, shared-secret authentication, HTTP API authentication, rate limiting, and rotation runbooks.

## 10. Observability

### 10.1 Metrics (Prometheus Format)

```
# Cache metrics
gossipcache_hits_total{node_id}
gossipcache_misses_total{node_id}
gossipcache_evictions_total{node_id}
gossipcache_size_bytes{node_id}
gossipcache_keys{node_id}

# Gossip metrics
gossipcache_gossip_messages_sent_total{node_id}
gossipcache_gossip_messages_received_total{node_id}
gossipcache_gossip_latency_seconds{node_id,quantile}

# Peer metrics
gossipcache_peers{node_id,status}
gossipcache_peer_last_seen_seconds{node_id,peer_id}

# Backing store metrics (backed mode)
gossipcache_backing_store_latency_seconds{operation,quantile}
gossipcache_backing_store_errors_total{operation}
```

### 10.2 Logging

Structured logging using standard library `log/slog`:

```go
logger.Info("gossip_sent",
    "node_id", nodeID,
    "peer", peerID,
    "message_type", msgType,
    "keys", keyCount)
```

### 10.3 Tracing (Optional)

OpenTelemetry integration for distributed tracing.

### 10.4 Debug and Profiling Endpoints

Production builds should support configurable operational endpoints:

- `/debug/peers` for peer membership state
- `/debug/gossip` for gossip queue and protocol counters
- `/debug/cache` for bounded cache samples
- `/debug/pprof/*` for CPU, heap, goroutine, and trace profiling

These endpoints must be disabled by default or bound only to trusted interfaces.

## 11. Testing Strategy

### 11.1 Unit Tests
- All public interfaces
- Core algorithms (vector clocks, merkle trees)
- Target: > 80% code coverage

### 11.2 Integration Tests
- Backed mode with Redis/Postgres
- Multi-node clusters (3, 5, 10 nodes)
- Network failures, partitions

### 11.3 Performance Tests
- Benchmarks for all operations
- Load testing (sustained throughput)
- Stress testing (max cluster size)

### 11.4 Chaos Testing
- Random node failures
- Network partitions
- Backing store failures
- Clock skew

## 12. Future Considerations

### 12.1 Phase 2 Features
- Sharding/partial replication
- Multi-datacenter support
- Query/filter operations
- Compression for large values

### 12.2 Phase 3 Features
- Persistence layer for independent mode
- Built-in observability UI
- Dynamic reconfiguration
- Read-your-writes consistency option

## 13. References

- [ARCHITECTURE.md](ARCHITECTURE.md) - High-level architecture
- [BACKED_MODE_SEQUENCES.md](diagrams/BACKED_MODE_SEQUENCES.md) - Backed mode flows
- [INDEPENDENT_MODE_SEQUENCES.md](diagrams/INDEPENDENT_MODE_SEQUENCES.md) - Independent mode flows
- [DEPLOYMENT.md](DEPLOYMENT.md) - Deployment guides

## 14. Glossary

- **Gossip**: Peer-to-peer communication protocol where nodes randomly exchange state
- **Vector Clock**: Logical clock for tracking causality in distributed systems
- **Anti-Entropy**: Process of synchronizing state between nodes to repair inconsistencies
- **Tombstone**: Marker for deleted keys to prevent resurrection
- **Singleflight**: Pattern to prevent duplicate work when multiple requests for same key arrive
- **Merkle Tree**: Hash tree for efficient comparison of large datasets
- **TTL**: Time-to-live, expiration time for cached entries

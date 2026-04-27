# Phase 3: Independent Mode Implementation

**Goal**: Implement independent mode with vector clocks, conflict detection, and full-data gossip.

**Duration**: 3-4 weeks

**Prerequisites**: Phase 2 complete

**Status**: Not Started

## Overview

Phase 3 implements independent mode where GossipCache operates without a backing store. This requires vector clocks for conflict detection, full-data gossip protocol, and sophisticated conflict resolution strategies.

## Objectives

- [ ] Vector clock implementation
- [ ] Full-data gossip protocol (includes values)
- [ ] Conflict detection using vector clocks
- [ ] Conflict resolution strategies (LWW, custom merge, siblings)
- [ ] Tombstone handling for deletes
- [ ] Anti-entropy for independent mode
- [ ] Network partition tolerance
- [ ] Integration tests with partition scenarios
- [ ] Chaos testing

## Architecture Reference

Review these documents:
- [Independent Mode Sequences](../diagrams/INDEPENDENT_MODE_SEQUENCES.md)
- [Architecture - Independent Mode](../ARCHITECTURE.md#independent-mode)
- [Technical Spec - Vector Clocks](../TECHNICAL_SPEC.md#531-vector-clock-comparison)

## Package Structure Updates

```
gossipcache/
├── internal/
│   ├── vclock/
│   │   ├── vclock.go              # Vector clock implementation
│   │   ├── comparator.go          # Clock comparison logic
│   │   └── vclock_test.go
│   ├── conflict/
│   │   ├── resolver.go            # Conflict resolution interface
│   │   ├── lww.go                 # Last-write-wins
│   │   ├── custom.go              # Custom merge functions
│   │   └── siblings.go            # Keep both (siblings)
│   ├── gossip/
│   │   ├── dataupdate.go          # Full data gossip messages
│   │   └── independent_engine.go  # Independent mode gossip
│   └── storage/
│       └── memory/
│           └── versioned.go       # Storage with vector clocks
└── test/
    ├── integration/
    │   └── independent_mode_test.go
    └── chaos/
        └── partition_test.go
```

## Implementation Steps

### Step 1: Vector Clock Implementation (Day 1-3)

**SOLID**: Single Responsibility - Vector clock handles only causality tracking

```go
// internal/vclock/vclock.go
package vclock

import (
    "bytes"
    "encoding/gob"
    "sort"
)

// VectorClock tracks causality in distributed systems
type VectorClock map[string]int64

// New creates a new vector clock
func New() VectorClock {
    return make(VectorClock)
}

// Increment increments the clock for a node
func (vc VectorClock) Increment(nodeID string) {
    vc[nodeID]++
}

// Update updates the clock with values from another clock
func (vc VectorClock) Update(other VectorClock) {
    for nodeID, count := range other {
        if vc[nodeID] < count {
            vc[nodeID] = count
        }
    }
}

// Copy creates a deep copy
func (vc VectorClock) Copy() VectorClock {
    copy := make(VectorClock, len(vc))
    for k, v := range vc {
        copy[k] = v
    }
    return copy
}

// Bytes serializes the vector clock
func (vc VectorClock) Bytes() ([]byte, error) {
    var buf bytes.Buffer
    enc := gob.NewEncoder(&buf)
    if err := enc.Encode(vc); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

// FromBytes deserializes a vector clock
func FromBytes(data []byte) (VectorClock, error) {
    var vc VectorClock
    buf := bytes.NewBuffer(data)
    dec := gob.NewDecoder(buf)
    if err := dec.Decode(&vc); err != nil {
        return nil, err
    }
    return vc, nil
}

// String returns a string representation
func (vc VectorClock) String() string {
    // Sort for consistent output
    keys := make([]string, 0, len(vc))
    for k := range vc {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    var buf bytes.Buffer
    buf.WriteString("{")
    for i, k := range keys {
        if i > 0 {
            buf.WriteString(", ")
        }
        buf.WriteString(k)
        buf.WriteString(":")
        buf.WriteString(fmt.Sprintf("%d", vc[k]))
    }
    buf.WriteString("}")
    return buf.String()
}
```

### Step 2: Vector Clock Comparator (Day 3-4)

```go
// internal/vclock/comparator.go
package vclock

// Relation describes the relationship between two vector clocks
type Relation int

const (
    Equal        Relation = iota // Clocks are equal
    LocalNewer                   // Local is strictly newer
    RemoteNewer                  // Remote is strictly newer
    Concurrent                   // Clocks are concurrent (conflict!)
)

// Compare compares two vector clocks
func Compare(local, remote VectorClock) Relation {
    localGreater := false
    remoteGreater := false

    // Get all unique node IDs
    allNodes := make(map[string]bool)
    for nodeID := range local {
        allNodes[nodeID] = true
    }
    for nodeID := range remote {
        allNodes[nodeID] = true
    }

    // Compare each component
    for nodeID := range allNodes {
        localVal := local[nodeID]
        remoteVal := remote[nodeID]

        if localVal > remoteVal {
            localGreater = true
        } else if remoteVal > localVal {
            remoteGreater = true
        }
    }

    // Determine relation
    if localGreater && !remoteGreater {
        return LocalNewer
    } else if remoteGreater && !localGreater {
        return RemoteNewer
    } else if localGreater && remoteGreater {
        return Concurrent
    } else {
        return Equal
    }
}

// Merge creates a new vector clock with the maximum of each component
func Merge(local, remote VectorClock) VectorClock {
    merged := make(VectorClock)

    allNodes := make(map[string]bool)
    for nodeID := range local {
        allNodes[nodeID] = true
    }
    for nodeID := range remote {
        allNodes[nodeID] = true
    }

    for nodeID := range allNodes {
        localVal := local[nodeID]
        remoteVal := remote[nodeID]

        if localVal > remoteVal {
            merged[nodeID] = localVal
        } else {
            merged[nodeID] = remoteVal
        }
    }

    return merged
}
```

### Step 3: Conflict Resolution Strategies (Day 5-7)

**SOLID**: Open/Closed - New resolution strategies can be added without modifying existing code

```go
// internal/conflict/resolver.go
package conflict

import (
    "github.com/sanketn26/gossipcache/internal/storage"
    "github.com/sanketn26/gossipcache/internal/vclock"
)

// Resolver defines the interface for conflict resolution
type Resolver interface {
    Resolve(local, remote *VersionedEntry) (*VersionedEntry, error)
}

type VersionedEntry struct {
    *storage.Entry
    VectorClock vclock.VectorClock
}

// Strategy types
type Strategy string

const (
    LastWriteWins Strategy = "last_write_wins"
    CustomMerge   Strategy = "custom_merge"
    KeepSiblings  Strategy = "keep_siblings"
)

// NewResolver creates a resolver based on strategy
func NewResolver(strategy Strategy, mergeFn MergeFunc) Resolver {
    switch strategy {
    case LastWriteWins:
        return &LWWResolver{}
    case CustomMerge:
        return &CustomResolver{mergeFn: mergeFn}
    case KeepSiblings:
        return &SiblingsResolver{}
    default:
        return &LWWResolver{}
    }
}

type MergeFunc func(local, remote []byte) ([]byte, error)
```

```go
// internal/conflict/lww.go
package conflict

import (
    "github.com/sanketn26/gossipcache/internal/vclock"
)

// LWWResolver implements last-write-wins conflict resolution
type LWWResolver struct{}

func (r *LWWResolver) Resolve(local, remote *VersionedEntry) (*VersionedEntry, error) {
    // Compare timestamps (wall clock)
    if remote.UpdatedAt.After(local.UpdatedAt) {
        // Remote wins, but merge vector clocks
        return &VersionedEntry{
            Entry:       remote.Entry,
            VectorClock: vclock.Merge(local.VectorClock, remote.VectorClock),
        }, nil
    }

    // Local wins, but merge vector clocks
    return &VersionedEntry{
        Entry:       local.Entry,
        VectorClock: vclock.Merge(local.VectorClock, remote.VectorClock),
    }, nil
}
```

### Step 4: Full-Data Gossip Messages (Day 7-9)

```go
// internal/gossip/dataupdate.go
package gossip

import (
    "github.com/sanketn26/gossipcache/internal/vclock"
)

// DataUpdate carries full data for independent mode
type DataUpdate struct {
    Key         string
    Value       []byte
    VectorClock vclock.VectorClock
    TTL         time.Duration
    Timestamp   time.Time
    NodeID      string
    Tombstone   bool // true if this is a delete
}

func (m *DataUpdate) Type() MessageType {
    return MsgDataUpdate
}

func (m *DataUpdate) Serialize() ([]byte, error) {
    // Use gob or protobuf
    // Include key, value, vector clock, TTL, etc.
    var buf bytes.Buffer
    enc := gob.NewEncoder(&buf)

    if err := enc.Encode(m); err != nil {
        return nil, err
    }

    return buf.Bytes(), nil
}

func DeserializeDataUpdate(data []byte) (*DataUpdate, error) {
    var msg DataUpdate
    buf := bytes.NewBuffer(data)
    dec := gob.NewDecoder(buf)

    if err := dec.Decode(&msg); err != nil {
        return nil, err
    }

    return &msg, nil
}
```

### Step 5: Independent Mode Gossip Engine (Day 10-15)

```go
// internal/gossip/independent_engine.go
package gossip

import (
    "context"

    "github.com/sanketn26/gossipcache/internal/conflict"
    "github.com/sanketn26/gossipcache/internal/storage"
    "github.com/sanketn26/gossipcache/internal/vclock"
)

// IndependentEngine implements gossip for independent mode
type IndependentEngine struct {
    *Engine // Embed base engine
    resolver conflict.Resolver
}

func NewIndependentEngine(
    nodeID string,
    transport *network.Transport,
    storage storage.Storage,
    resolver conflict.Resolver,
    config *Config,
) *IndependentEngine {
    base := &Engine{
        nodeID:    nodeID,
        transport: transport,
        storage:   storage,
        peers:     NewPeerManager(),
        config:    config,
        closed:    make(chan struct{}),
    }

    ie := &IndependentEngine{
        Engine:   base,
        resolver: resolver,
    }

    // Register handlers for independent mode
    transport.RegisterHandler(MsgDataUpdate, ie.handleDataUpdate)

    return ie
}

// BroadcastData sends full data to peers (independent mode)
func (ie *IndependentEngine) BroadcastData(
    ctx context.Context,
    key string,
    value []byte,
    vectorClock vclock.VectorClock,
    ttl time.Duration,
) error {
    msg := &DataUpdate{
        Key:         key,
        Value:       value,
        VectorClock: vectorClock,
        TTL:         ttl,
        Timestamp:   time.Now(),
        NodeID:      ie.nodeID,
        Tombstone:   false,
    }

    // Gossip to random peers
    peers := ie.selectGossipPeers()

    for _, peer := range peers {
        go func(p *Peer) {
            ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
            defer cancel()

            ie.transport.SendUDP(ctx, p.Address, msg)
        }(peer)
    }

    return nil
}

func (ie *IndependentEngine) handleDataUpdate(msg Message, from net.Addr) error {
    update, ok := msg.(*DataUpdate)
    if !ok {
        return fmt.Errorf("invalid message type")
    }

    ctx := context.Background()

    // Get local entry with vector clock
    localEntry, localVClock, err := ie.getLocalEntryWithVClock(ctx, update.Key)

    if err != nil || localEntry == nil {
        // No local entry, accept remote
        return ie.applyUpdate(ctx, update)
    }

    // Compare vector clocks
    relation := vclock.Compare(localVClock, update.VectorClock)

    switch relation {
    case vclock.RemoteNewer:
        // Remote is strictly newer, accept
        return ie.applyUpdate(ctx, update)

    case vclock.LocalNewer:
        // Local is newer, ignore
        return nil

    case vclock.Concurrent:
        // Conflict! Resolve it
        return ie.resolveConflict(ctx, localEntry, localVClock, update)

    case vclock.Equal:
        // Same version, ignore
        return nil
    }

    return nil
}

func (ie *IndependentEngine) resolveConflict(
    ctx context.Context,
    localEntry *storage.Entry,
    localVClock vclock.VectorClock,
    update *DataUpdate,
) error {
    local := &conflict.VersionedEntry{
        Entry:       localEntry,
        VectorClock: localVClock,
    }

    remote := &conflict.VersionedEntry{
        Entry: &storage.Entry{
            Key:       update.Key,
            Value:     update.Value,
            UpdatedAt: update.Timestamp,
        },
        VectorClock: update.VectorClock,
    }

    // Resolve using configured strategy
    resolved, err := ie.resolver.Resolve(local, remote)
    if err != nil {
        return err
    }

    // Apply resolved entry
    return ie.applyResolvedEntry(ctx, resolved)
}

func (ie *IndependentEngine) getLocalEntryWithVClock(
    ctx context.Context,
    key string,
) (*storage.Entry, vclock.VectorClock, error) {
    // This requires extending storage to store vector clocks
    // For now, return entry with empty vclock
    entry, err := ie.storage.Get(ctx, key)
    if err != nil {
        return nil, nil, err
    }

    // TODO: Retrieve stored vector clock
    vc := vclock.New()

    return entry, vc, nil
}

func (ie *IndependentEngine) applyUpdate(ctx context.Context, update *DataUpdate) error {
    if update.Tombstone {
        return ie.storage.Delete(ctx, update.Key)
    }

    return ie.storage.Set(ctx, update.Key, update.Value, update.TTL)
    // TODO: Also store vector clock
}

func (ie *IndependentEngine) applyResolvedEntry(
    ctx context.Context,
    entry *conflict.VersionedEntry,
) error {
    return ie.storage.Set(ctx, entry.Key, entry.Value, 0)
    // TODO: Store resolved vector clock
}
```

### Step 6: Storage Extensions for Vector Clocks (Day 15-17)

Extend storage interface to support vector clock metadata.

### Step 7: Tombstone Handling (Day 17-18)

Implement tombstone mechanism for deletes in independent mode.

### Step 8: Integration and Chaos Tests (Day 19-21)

```go
// test/integration/independent_mode_test.go
package integration

func TestIndependentMode_ConflictResolution(t *testing.T) {
    // Setup 3-node cluster in independent mode
    nodes := setupIndependentCluster(t, 3)
    defer cleanupCluster(nodes)

    ctx := context.Background()

    // Partition network: isolate node 0
    partitionNetwork(t, nodes, []int{0}, []int{1, 2})

    // Concurrent writes on different partitions
    nodes[0].Set(ctx, "key1", []byte("value_from_0"), 0)
    nodes[1].Set(ctx, "key1", []byte("value_from_1"), 0)

    // Heal partition
    healNetwork(t, nodes)

    // Wait for gossip and conflict resolution
    time.Sleep(3 * time.Second)

    // Verify all nodes converged (LWW)
    for i, node := range nodes {
        value, err := node.Get(ctx, "key1")
        require.NoError(t, err)
        t.Logf("Node %d has value: %s", i, value)
    }

    // All should have same value after resolution
    val0, _ := nodes[0].Get(ctx, "key1")
    val1, _ := nodes[1].Get(ctx, "key1")
    val2, _ := nodes[2].Get(ctx, "key1")

    require.Equal(t, val0, val1)
    require.Equal(t, val1, val2)
}
```

```go
// test/chaos/partition_test.go
package chaos

func TestChaos_NetworkPartition(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping chaos test")
    }

    // Long-running test with random partitions
    nodes := setupCluster(t, 5)
    defer cleanupCluster(nodes)

    // Run for 5 minutes with random partitions
    duration := 5 * time.Minute
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    timeout := time.After(duration)

    for {
        select {
        case <-timeout:
            // Test complete, verify consistency
            verifyEventualConsistency(t, nodes)
            return

        case <-ticker.C:
            // Random partition
            createRandomPartition(t, nodes)

            // Random operations
            performRandomOperations(t, nodes, 100)

            // Heal after some time
            time.Sleep(10 * time.Second)
            healNetwork(t, nodes)
        }
    }
}
```

## Deliverables

- [ ] Vector clock implementation with comparison
- [ ] Conflict resolution strategies (LWW, custom, siblings)
- [ ] Full-data gossip protocol
- [ ] Tombstone handling
- [ ] Independent mode cache manager
- [ ] Integration tests with conflicts
- [ ] Chaos tests with partitions
- [ ] Performance benchmarks

## Success Criteria

1. **Functional**: Independent mode works without backing store
2. **Conflict Handling**: Conflicts detected and resolved correctly
3. **Partition Tolerance**: System handles network partitions gracefully
4. **Quality**: >80% coverage, chaos tests passing

## Next Phase

Move to [Phase 4: Production Readiness](PHASE_4_PRODUCTION.md) for additional backing stores, deployment features, and operational tooling.

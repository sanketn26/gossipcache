# Independent Mode Sequence Diagrams

## Overview

Independent mode operates without a backing store. All data is stored in-memory across the cluster, and gossip protocol propagates full data (not just metadata). This mode uses vector clocks for conflict detection and resolution.

## 1. Cache Read - Hit

```mermaid
sequenceDiagram
    participant App as Application
    participant Node as Cache Node
    participant Store as Local Storage

    App->>Node: Get(key)
    Node->>Store: Lookup(key)
    Store-->>Node: value, vector_clock

    alt Value exists and not expired
        Node-->>App: value (< 1ms)
    else Key not found
        Node-->>App: ❌ Not Found
    end
```

## 2. Cache Read - Miss (No Backing Store)

```mermaid
sequenceDiagram
    participant App as Application
    participant Node as Cache Node
    participant Store as Local Storage

    App->>Node: Get(key)
    Node->>Store: Lookup(key)
    Store-->>Node: Not Found

    Note over Node: No backing store in independent mode

    Node-->>App: ❌ Not Found (or default value)

    Note over App: Application must handle miss<br/>(compute, fetch from primary source, etc.)
```

## 3. Cache Write (Single Node)

```mermaid
sequenceDiagram
    participant App as Application
    participant Node1 as Cache Node 1
    participant Store1 as Local Storage
    participant VClock as Vector Clock<br/>Manager
    participant Gossip as Gossip Engine
    participant Node2 as Cache Node 2
    participant Node3 as Cache Node 3

    App->>Node1: Set(key, value)

    Node1->>VClock: IncrementClock(node1)
    VClock-->>Node1: new_vclock {node1: 5, node2: 2}

    Node1->>Store1: Set(key, value, new_vclock)
    Node1-->>App: OK

    Note over Node1,Gossip: Async gossip propagation

    Node1->>Gossip: Broadcast DataUpdate
    Note over Gossip: Message: {key, value, vclock}

    par Gossip to Peers (Full Data)
        Gossip->>Node2: DataUpdate<br/>{key, value, vclock}
        Gossip->>Node3: DataUpdate<br/>{key, value, vclock}
    end
```

## 4. Gossip Data Propagation (No Conflicts)

```mermaid
sequenceDiagram
    participant Node1 as Cache Node 1<br/>(Writer)
    participant Node2 as Cache Node 2<br/>(Receiver)
    participant Store2 as Local Storage
    participant VClock as Vector Clock<br/>Comparator

    Node1->>Node2: DataUpdate<br/>{key: "session:abc", value: "data", vclock: {n1:5, n2:2}}

    Node2->>Store2: GetVectorClock(key)
    Store2-->>Node2: local_vclock: {n1:4, n2:2}

    Node2->>VClock: Compare(local_vclock, remote_vclock)
    VClock-->>Node2: REMOTE_NEWER

    Note over Node2: Remote is strictly newer<br/>No conflict, safe to update

    Node2->>Store2: Set(key, value, new_vclock)

    Note over Node2: Update complete, no merge needed
```

## 5. Concurrent Writes (Conflict Detection)

```mermaid
sequenceDiagram
    participant App1 as Application 1
    participant Node1 as Cache Node 1
    participant Store1 as Local Storage
    participant App2 as Application 2
    participant Node2 as Cache Node 2
    participant Store2 as Local Storage
    participant Node3 as Cache Node 3

    par Concurrent Writes
        App1->>Node1: Set(key, "value1")
        Node1->>Store1: Set(key, "value1", {n1:5, n2:2})
    and
        App2->>Node2: Set(key, "value2")
        Node2->>Store2: Set(key, "value2", {n1:4, n2:3})
    end

    Node1-->>App1: OK
    Node2-->>App2: OK

    Note over Node1,Node2: Both updated local caches<br/>Vector clocks diverged

    par Gossip Propagation
        Node1->>Node3: DataUpdate<br/>{key, "value1", {n1:5, n2:2}}
        Node2->>Node3: DataUpdate<br/>{key, "value2", {n1:4, n2:3}}
    end

    Note over Node3: Receives conflicting updates<br/>Vector clocks are concurrent<br/>(neither is ancestor of other)

    Node3->>Node3: Detect Conflict<br/>Compare {n1:5, n2:2} vs {n1:4, n2:3}

    Note over Node3: Conflict resolution strategy
```

## 6. Conflict Resolution Strategies

### 6.1 Last-Write-Wins (LWW)

```mermaid
sequenceDiagram
    participant Node3 as Cache Node 3
    participant Store3 as Local Storage
    participant Resolver as Conflict Resolver<br/>(LWW Strategy)

    Note over Node3: Conflict detected<br/>value1: {n1:5, n2:2, ts:1000}<br/>value2: {n1:4, n2:3, ts:1001}

    Node3->>Resolver: Resolve(value1, value2)

    Resolver->>Resolver: Compare timestamps<br/>1001 > 1000

    Resolver-->>Node3: Winner: value2

    Node3->>Store3: Set(key, "value2", merged_vclock: {n1:5, n2:3})

    Note over Node3: Merged vector clock:<br/>Take max of each component
```

### 6.2 Custom Merge Strategy

```mermaid
sequenceDiagram
    participant Node3 as Cache Node 3
    participant Store3 as Local Storage
    participant Resolver as Conflict Resolver<br/>(Custom Merge)

    Note over Node3: Conflict: counter values<br/>value1: {count: 100}<br/>value2: {count: 150}

    Node3->>Resolver: Resolve(value1, value2, merge_func)

    Resolver->>Resolver: Apply merge function<br/>(e.g., sum for counters)

    Resolver-->>Node3: Merged: {count: 250}

    Node3->>Store3: Set(key, merged_value, merged_vclock)

    Note over Node3: Application-specific merge logic
```

### 6.3 Keep Both (Siblings)

```mermaid
sequenceDiagram
    participant Node3 as Cache Node 3
    participant Store3 as Local Storage
    participant App as Application

    Note over Node3: Conflict: incompatible values<br/>Cannot auto-resolve

    Node3->>Store3: SetSiblings(key, [value1, value2], vclocks)

    App->>Node3: Get(key)
    Node3->>Store3: Lookup(key)
    Store3-->>Node3: [value1, value2] (siblings)

    Node3-->>App: Multiple values (conflict)

    App->>App: Application resolves conflict

    App->>Node3: Set(key, resolved_value)

    Note over Node3: Application provides canonical version
```

## 7. Network Partition & Healing

### 7.1 During Partition

```mermaid
sequenceDiagram
    participant Node1 as Cache Node 1<br/>(Partition A)
    participant Node2 as Cache Node 2<br/>(Partition A)
    participant Node3 as Cache Node 3<br/>(Partition B)
    participant Node4 as Cache Node 4<br/>(Partition B)

    Note over Node1,Node2,Node3,Node4: Network partition occurs

    rect rgb(255, 200, 200)
        Note over Node1,Node2: Partition A<br/>Nodes 1, 2 can communicate
    end

    rect rgb(200, 200, 255)
        Note over Node3,Node4: Partition B<br/>Nodes 3, 4 can communicate
    end

    par Operations in Partition A
        Node1->>Node1: Set(key, "valueA")
        Node1->>Node2: Gossip update
    and Operations in Partition B
        Node3->>Node3: Set(key, "valueB")
        Node3->>Node4: Gossip update
    end

    Note over Node1,Node4: Partitions operate independently<br/>Divergent state accumulates
```

### 7.2 Partition Healing

```mermaid
sequenceDiagram
    participant Node1 as Cache Node 1<br/>(Had valueA)
    participant Node3 as Cache Node 3<br/>(Had valueB)
    participant Store1 as Local Storage
    participant VClock as Vector Clock<br/>Comparator

    Note over Node1,Node3: Network partition heals

    Node1->>Node3: DataUpdate<br/>{key, "valueA", vclock_A}
    Node3->>Node1: DataUpdate<br/>{key, "valueB", vclock_B}

    par Conflict Detection
        Node1->>VClock: Compare(local_vclock_B, remote_vclock_A)
        VClock-->>Node1: CONCURRENT (conflict)
    and
        Node3->>VClock: Compare(local_vclock_A, remote_vclock_B)
        VClock-->>Node3: CONCURRENT (conflict)
    end

    Note over Node1,Node3: Both nodes detect conflict

    par Conflict Resolution
        Node1->>Node1: Resolve(valueA, valueB)<br/>→ Winner: valueB (example)
        Node1->>Store1: Update(key, "valueB", merged_vclock)
    and
        Node3->>Node3: Resolve(valueB, valueA)<br/>→ Winner: valueB (same)
    end

    Note over Node1,Node3: Deterministic resolution ensures<br/>both nodes converge to same value
```

## 8. Anti-Entropy Process

```mermaid
sequenceDiagram
    participant Node1 as Cache Node 1
    participant Store1 as Local Storage
    participant Node2 as Cache Node 2
    participant Store2 as Local Storage

    Note over Node1,Node2: Periodic anti-entropy<br/>(e.g., every 5 minutes)

    Node1->>Store1: GetAllKeysDigest()
    Store1-->>Node1: Merkle tree root

    Node1->>Node2: AntiEntropyRequest<br/>(merkle_root, key_count)

    Node2->>Store2: GetAllKeysDigest()
    Store2-->>Node2: Merkle tree root

    Node2->>Node2: Compare merkle roots

    alt Roots Match
        Note over Node2: All data synchronized
        Node2-->>Node1: OK (no sync needed)
    else Roots Differ
        Node2-->>Node1: RequestSubtreeDiff

        Node1->>Node2: Send differing keys & values & vclocks

        Node2->>Node2: For each key:<br/>Compare vector clocks<br/>Update if remote newer or resolve conflicts

        Node2->>Store2: Update multiple keys
    end

    Note over Node1,Node2: Nodes now synchronized
```

## 9. Node Join - Cluster Bootstrap

```mermaid
sequenceDiagram
    participant NewNode as New Cache Node
    participant Discovery as Discovery Service
    participant Node1 as Existing Node 1
    participant Node2 as Existing Node 2

    NewNode->>Discovery: FindPeers()
    Discovery-->>NewNode: [Node1, Node2, ...]

    NewNode->>Node1: JoinRequest<br/>(initial_vclock: {newNode: 0})

    Node1-->>NewNode: JoinAck<br/>(cluster_metadata, peer_list)

    Note over NewNode: Start with empty cache<br/>Vector clock: {newNode: 0}

    NewNode->>Node2: JoinRequest
    Node2-->>NewNode: JoinAck

    Note over NewNode: Begin anti-entropy immediately

    NewNode->>Node1: AntiEntropyRequest<br/>(empty cache digest)

    Node1->>NewNode: AntiEntropyResponse<br/>(all keys, values, vclocks)

    NewNode->>NewNode: Populate cache from sync

    Note over NewNode: Now has full replica of cluster data
```

## 10. Node Failure & Recovery

```mermaid
sequenceDiagram
    participant Node1 as Cache Node 1
    participant Node2 as Cache Node 2<br/>(Fails)
    participant Node3 as Cache Node 3
    participant Node4 as Cache Node 4

    Note over Node2: Node 2 crashes

    Node1->>Node2: Gossip message
    Note over Node1: Timeout, no response

    Node1->>Node1: Mark Node2 as suspected

    Node1->>Node3: Node2 suspect notification
    Node1->>Node4: Node2 suspect notification

    Note over Node1,Node4: Continue operations without Node2<br/>Data on Node2 temporarily unavailable

    rect rgb(200, 255, 200)
        Note over Node2: Node 2 recovers
    end

    Node2->>Node1: JoinRequest (rejoin)
    Node1-->>Node2: Welcome back

    Node2->>Node1: AntiEntropyRequest<br/>(may have stale data)

    Node1->>Node2: AntiEntropyResponse<br/>(updated keys since failure)

    Node2->>Node2: Merge updates<br/>Resolve any conflicts

    Note over Node2: Node fully recovered
```

## 11. TTL-Based Expiration

```mermaid
sequenceDiagram
    participant App as Application
    participant Node1 as Cache Node 1
    participant Store1 as Local Storage
    participant Gossip as Gossip Engine
    participant Node2 as Cache Node 2

    App->>Node1: Set(key, value, ttl=60s)

    Node1->>Store1: Set(key, value, vclock, expires_at=now+60s)

    Node1->>Gossip: DataUpdate<br/>{key, value, vclock, ttl=60s}

    Gossip->>Node2: DataUpdate

    Note over Node1,Node2: Time passes... 60 seconds

    par TTL Expiration
        Node1->>Node1: Background expiration scan
        Node1->>Store1: Delete expired keys
    and
        Node2->>Node2: Background expiration scan
        Node2->>Store1: Delete expired keys
    end

    Note over Node1,Node2: Key expired on all nodes<br/>No gossip needed for deletions
```

## 12. Explicit Delete Operation

```mermaid
sequenceDiagram
    participant App as Application
    participant Node1 as Cache Node 1
    participant Store1 as Local Storage
    participant Gossip as Gossip Engine
    participant Node2 as Cache Node 2
    participant Store2 as Local Storage

    App->>Node1: Delete(key)

    Node1->>Store1: Get(key)
    Store1-->>Node1: current_vclock

    Node1->>Node1: Increment vector clock<br/>Mark as tombstone

    Node1->>Store1: Set(key, TOMBSTONE, new_vclock)
    Node1-->>App: OK

    Node1->>Gossip: DataUpdate<br/>{key, TOMBSTONE, vclock}

    Gossip->>Node2: DataUpdate

    Node2->>Store2: Set(key, TOMBSTONE, vclock)

    Note over Node2: Tombstone prevents resurrection<br/>of deleted key from anti-entropy

    Note over Node1,Node2: After TTL (e.g., 24h),<br/>tombstones garbage collected
```

## Key Characteristics

**Independent Mode Trade-offs:**
- ✅ Zero external dependencies
- ✅ Lower operational complexity
- ✅ Fast writes (no backing store latency)
- ✅ High availability during partitions
- ⚠️ Higher gossip bandwidth (full data)
- ⚠️ Limited by node memory (no backing store)
- ⚠️ Data loss if all replicas lost
- ⚠️ Conflicts require resolution logic

**Design Considerations:**
1. **Vector Clocks**: Accurate conflict detection requires maintaining clocks
2. **Conflict Resolution**: Must be deterministic across all nodes
3. **Tombstones**: Necessary to prevent deleted data resurrection
4. **Anti-Entropy**: Critical for healing partitions and new nodes
5. **Data Size**: Keep values small since gossip carries full data
6. **Replication Factor**: Ensure enough nodes for desired durability

**Best Use Cases:**
- Session storage
- Feature flags
- Service discovery
- Distributed rate limiting
- Configuration caching
- Ephemeral shared state

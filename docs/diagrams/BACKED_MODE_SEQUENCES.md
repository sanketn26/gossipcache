# Backed Mode Sequence Diagrams

## Overview

Backed mode uses a backing store (Redis/Valkey/Postgres) as the source of truth. Gossip protocol propagates metadata only, and nodes pull actual data from the backing store when changes are detected.

## 1. Cache Read - Hit

```mermaid
sequenceDiagram
    participant App as Application
    participant Node as Cache Node
    participant Store as Local Storage

    App->>Node: Get(key)
    Node->>Store: Lookup(key)
    Store-->>Node: value, version

    alt Value is fresh (not expired)
        Node-->>App: value (< 1ms)
    else Value is stale (TTL expired)
        Note over Node: See "Cache Read - Miss (TTL Expired)"
    end
```

## 2. Cache Read - Miss (Cold Start)

```mermaid
sequenceDiagram
    participant App as Application
    participant Node as Cache Node
    participant Store as Local Storage
    participant BS as Backing Store

    App->>Node: Get(key)
    Node->>Store: Lookup(key)
    Store-->>Node: Not Found

    Node->>BS: Get(key)
    BS-->>Node: value, version

    Node->>Store: Set(key, value, version)
    Node-->>App: value (backing store latency)
```

## 3. Cache Read - Miss (TTL Expired)

```mermaid
sequenceDiagram
    participant App as Application
    participant Node as Cache Node
    participant Store as Local Storage
    participant BS as Backing Store

    App->>Node: Get(key)
    Node->>Store: Lookup(key)
    Store-->>Node: value (expired), version

    Note over Node: TTL expired, refresh needed

    Node->>BS: Get(key)
    BS-->>Node: value, new_version

    Node->>Store: Update(key, value, new_version)
    Node-->>App: value
```

## 4. Cache Write (Single Node)

```mermaid
sequenceDiagram
    participant App as Application
    participant Node1 as Cache Node 1
    participant BS as Backing Store
    participant Store as Local Storage
    participant Gossip as Gossip Engine
    participant Node2 as Cache Node 2
    participant Node3 as Cache Node 3

    App->>Node1: Set(key, value)

    par Update Backing Store
        Node1->>BS: Set(key, value)
        BS-->>Node1: OK, version
    and Update Local Cache
        Node1->>Store: Set(key, value, version)
    end

    Node1-->>App: OK

    Note over Node1,Gossip: Async gossip propagation

    Node1->>Gossip: Broadcast ChangeNotification
    Note over Gossip: Message: {key, version, checksum}

    par Gossip to Peers
        Gossip->>Node2: ChangeNotification
        Gossip->>Node3: ChangeNotification
    end
```

## 5. Gossip Change Detection & Pull

```mermaid
sequenceDiagram
    participant Node1 as Cache Node 1<br/>(Writer)
    participant Node2 as Cache Node 2<br/>(Receiver)
    participant Store2 as Local Storage<br/>(Node 2)
    participant BS as Backing Store

    Node1->>Node2: ChangeNotification<br/>{key: "user:123", version: 5, checksum: "abc"}

    Node2->>Store2: CheckVersion(key)
    Store2-->>Node2: local_version: 3

    alt Version Mismatch (local < remote)
        Note over Node2: Local is stale, pull from backing store

        Node2->>BS: Get("user:123")
        BS-->>Node2: value, version: 5

        Node2->>Store2: Update(key, value, version: 5)

    else Version Match
        Note over Node2: Already up-to-date, ignore
    else Local Newer (shouldn't happen in backed mode)
        Note over Node2: Log anomaly, trust backing store
        Node2->>BS: Get("user:123")
        BS-->>Node2: value, version
        Node2->>Store2: Update(key, value, version)
    end
```

## 6. Concurrent Writes (Race Condition)

```mermaid
sequenceDiagram
    participant App1 as Application 1
    participant Node1 as Cache Node 1
    participant App2 as Application 2
    participant Node2 as Cache Node 2
    participant BS as Backing Store
    participant Node3 as Cache Node 3

    par Concurrent Writes
        App1->>Node1: Set(key, "value1")
        Node1->>BS: Set(key, "value1")
    and
        App2->>Node2: Set(key, "value2")
        Node2->>BS: Set(key, "value2")
    end

    Note over BS: Backing store handles conflict<br/>(last-write-wins, typically)

    BS-->>Node1: OK, version: 10
    BS-->>Node2: OK, version: 11

    par Gossip Propagation
        Node1->>Node3: ChangeNotification<br/>{key, version: 10}
        Node2->>Node3: ChangeNotification<br/>{key, version: 11}
    end

    Note over Node3: Receives both notifications

    Node3->>BS: Get(key)
    BS-->>Node3: "value2", version: 11

    Note over Node3: Backing store is source of truth<br/>Latest version wins
```

## 7. Backing Store Failure - Degraded Mode

```mermaid
sequenceDiagram
    participant App as Application
    participant Node as Cache Node
    participant Store as Local Storage
    participant BS as Backing Store

    App->>Node: Get(key)
    Node->>Store: Lookup(key)
    Store-->>Node: value (stale), version, last_updated

    Note over Node: Value exists but may be stale

    Node->>BS: Get(key)
    BS-->>Node: ❌ Connection Failed

    alt Within Staleness Threshold
        Note over Node: Serve stale with degraded flag
        Node-->>App: value (stale=true, age=10m)
    else Exceeds Staleness Threshold
        Note over Node: Too stale to serve
        Node-->>App: ❌ Error: Backing store unavailable
    end
```

## 8. Singleflight Pattern (Thundering Herd Prevention)

```mermaid
sequenceDiagram
    participant App1 as Application 1
    participant App2 as Application 2
    participant App3 as Application 3
    participant Node as Cache Node
    participant SF as Singleflight<br/>Controller
    participant Store as Local Storage
    participant BS as Backing Store

    par Concurrent Requests for Same Key
        App1->>Node: Get("popular_key")
        App2->>Node: Get("popular_key")
        App3->>Node: Get("popular_key")
    end

    Node->>Store: Lookup("popular_key")
    Store-->>Node: Not Found

    par Check Singleflight
        Node->>SF: RequestFlight("popular_key")
        Node->>SF: RequestFlight("popular_key")
        Node->>SF: RequestFlight("popular_key")
    end

    SF-->>Node: Flight 1: OK (you fetch)
    SF-->>Node: Flight 2: WAIT (sharing)
    SF-->>Node: Flight 3: WAIT (sharing)

    Note over Node: Only one goroutine fetches

    Node->>BS: Get("popular_key")
    BS-->>Node: value

    Node->>Store: Set("popular_key", value)

    SF->>SF: Broadcast result to waiters

    par Respond to All
        Node-->>App1: value
        Node-->>App2: value (shared fetch)
        Node-->>App3: value (shared fetch)
    end

    Note over Node,SF: Result: 1 backing store call instead of 3
```

## 9. Anti-Entropy Process

```mermaid
sequenceDiagram
    participant Node1 as Cache Node 1
    participant Store1 as Local Storage
    participant Node2 as Cache Node 2
    participant Store2 as Local Storage
    participant BS as Backing Store

    Note over Node1,Node2: Periodic anti-entropy (e.g., every 5 minutes)

    Node1->>Node2: AntiEntropyRequest<br/>(digest of all keys & versions)

    Node2->>Store2: GetAllVersions()
    Store2-->>Node2: {key1: v5, key2: v3, key3: v8}

    Node2->>Node2: Compare digests

    Note over Node2: Identifies differences:<br/>key1: Node1(v3) < Node2(v5)<br/>key2: Node1(v3) = Node2(v3)<br/>key4: Missing on Node2

    Node2-->>Node1: AntiEntropyResponse<br/>{stale_keys: [key1], missing_keys: [key4]}

    par Sync Stale Keys
        Node1->>BS: Get(key1)
        BS-->>Node1: value, v5
        Node1->>Store1: Update(key1, value, v5)
    and Sync Missing Keys
        Node2->>BS: Get(key4)
        BS-->>Node2: value, v2
        Node2->>Store2: Set(key4, value, v2)
    end

    Note over Node1,Node2: Both nodes now synchronized
```

## 10. Node Join - Bootstrap

```mermaid
sequenceDiagram
    participant NewNode as New Cache Node
    participant Discovery as Discovery Service<br/>(EC2/K8s/Docker)
    participant Node1 as Existing Node 1
    participant Node2 as Existing Node 2
    participant BS as Backing Store

    NewNode->>Discovery: FindPeers()
    Discovery-->>NewNode: [Node1, Node2, ...]

    par Join Cluster
        NewNode->>Node1: JoinRequest
        NewNode->>Node2: JoinRequest
    end

    Node1-->>NewNode: JoinAck (cluster metadata)
    Node2-->>NewNode: JoinAck

    Note over NewNode: Start with empty cache

    par Begin Gossip
        Node1->>NewNode: ChangeNotifications (ongoing)
        Node2->>NewNode: ChangeNotifications
    end

    Note over NewNode: Lazy population:<br/>Cache fills on-demand or via gossip

    alt On Cache Miss
        NewNode->>BS: Get(key)
        BS-->>NewNode: value, version
    end

    Note over NewNode: No bulk sync needed,<br/>cache warms up naturally
```

## Key Characteristics

**Backed Mode Trade-offs:**
- ✅ Minimal gossip bandwidth (metadata only)
- ✅ Scales to large values (gossip size constant)
- ✅ Backing store is source of truth (consistency)
- ✅ Graceful degradation (serve stale on failure)
- ⚠️ Extra network hop on cache miss/stale
- ⚠️ Dependency on backing store availability
- ⚠️ Eventual consistency (staleness window)

**Optimization Strategies:**
1. **Singleflight**: Prevent thundering herd on popular keys
2. **TTL Tuning**: Balance freshness vs backing store load
3. **Gossip Interval**: Lower interval = fresher data, higher overhead
4. **Staleness Threshold**: How long to serve stale in degraded mode

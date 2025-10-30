# Component Interaction Diagrams

## Overview

This document provides detailed component interaction diagrams showing how different parts of GossipCache work together.

## 1. Component Hierarchy

```
┌─────────────────────────────────────────────────────────────────────┐
│                          GossipCache System                          │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                      Application Layer                       │   │
│  │              (Go Client Library / HTTP API)                  │   │
│  └───────────────────────────┬─────────────────────────────────┘   │
│                              │                                       │
│  ┌───────────────────────────▼─────────────────────────────────┐   │
│  │                      Cache Manager                           │   │
│  │  • Request routing                                           │   │
│  │  • Mode selection                                            │   │
│  │  • Coordination                                              │   │
│  └───────────────────────────┬─────────────────────────────────┘   │
│                              │                                       │
│          ┌───────────────────┼───────────────────┐                  │
│          │                   │                   │                  │
│  ┌───────▼────────┐  ┌──────▼─────────┐  ┌─────▼──────────┐       │
│  │ Local Storage  │  │ Backing Store  │  │ Gossip Engine  │       │
│  │    Engine      │  │   Connector    │  │                │       │
│  └────────────────┘  └────────────────┘  └────────┬───────┘       │
│                                                     │                │
│                                          ┌──────────▼──────────┐    │
│                                          │ Network & Discovery │    │
│                                          └─────────────────────┘    │
└─────────────────────────────────────────────────────────────────────┘
```

## 2. Read Operation Flow (Backed Mode)

```
┌────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  App   │     │    Cache     │     │    Local     │     │   Backing    │
│        │     │   Manager    │     │   Storage    │     │    Store     │
└───┬────┘     └──────┬───────┘     └──────┬───────┘     └──────┬───────┘
    │                 │                     │                     │
    │ Get(key)        │                     │                     │
    ├────────────────>│                     │                     │
    │                 │                     │                     │
    │                 │ Lookup(key)         │                     │
    │                 ├────────────────────>│                     │
    │                 │                     │                     │
    │                 │ Check TTL           │                     │
    │                 │<────────────────────┤                     │
    │                 │                     │                     │
    │                 │  [Cache Hit & Fresh]│                     │
    │                 │◄────────────────────┤                     │
    │                 │                     │                     │
    │  value          │                     │                     │
    │<────────────────┤                     │                     │
    │                 │                     │                     │
    │                 │   [Cache Miss/Stale]│                     │
    │                 │                     │                     │
    │                 │ Get(key)            │                     │
    │                 ├─────────────────────┼────────────────────>│
    │                 │                     │                     │
    │                 │                     │ value, version      │
    │                 │<────────────────────┼─────────────────────┤
    │                 │                     │                     │
    │                 │ Update(key, value, version)               │
    │                 ├────────────────────>│                     │
    │                 │                     │                     │
    │  value          │                     │                     │
    │<────────────────┤                     │                     │
    │                 │                     │                     │
```

## 3. Write Operation Flow (Backed Mode)

```
┌────────┐  ┌─────────┐  ┌──────────┐  ┌─────────┐  ┌────────────┐
│  App   │  │  Cache  │  │  Local   │  │ Backing │  │   Gossip   │
│        │  │ Manager │  │ Storage  │  │  Store  │  │   Engine   │
└───┬────┘  └────┬────┘  └────┬─────┘  └────┬────┘  └─────┬──────┘
    │            │             │             │             │
    │Set(k,v)    │             │             │             │
    ├───────────>│             │             │             │
    │            │             │             │             │
    │            │ Set(k,v)────┼────────────>│             │
    │            │             │             │             │
    │            │             │             │ OK,version  │
    │            │<────────────┼─────────────┤             │
    │            │             │             │             │
    │            │Set(k,v,ver) │             │             │
    │            ├────────────>│             │             │
    │            │             │             │             │
    │            │   OK        │             │             │
    │            │<────────────┤             │             │
    │            │             │             │             │
    │  OK        │             │             │             │
    │<───────────┤             │             │             │
    │            │             │             │             │
    │            │ Async: Broadcast ChangeNotification     │
    │            ├─────────────┼─────────────┼────────────>│
    │            │             │             │             │
    │            │             │             │             │ To Peers
    │            │             │             │             ├────────>
    │            │             │             │             │
```

## 4. Gossip Message Processing (Backed Mode)

```
┌──────────┐  ┌────────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│ Remote   │  │  Network   │  │  Gossip  │  │  Local   │  │ Backing  │
│   Node   │  │   Layer    │  │  Engine  │  │ Storage  │  │  Store   │
└────┬─────┘  └─────┬──────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │              │              │             │             │
     │ChangeNotif   │              │             │             │
     ├─────────────>│              │             │             │
     │              │              │             │             │
     │              │ Receive      │             │             │
     │              ├─────────────>│             │             │
     │              │              │             │             │
     │              │              │ GetVersion(key)           │
     │              │              ├────────────>│             │
     │              │              │             │             │
     │              │              │ local_ver   │             │
     │              │              │<────────────┤             │
     │              │              │             │             │
     │              │              │ Compare versions          │
     │              │              │                           │
     │              │              │ [If remote > local]       │
     │              │              │                           │
     │              │              │ Get(key) ─────────────────>│
     │              │              │                           │
     │              │              │ value, version            │
     │              │              │<──────────────────────────┤
     │              │              │                           │
     │              │              │ Update(key, value, ver)   │
     │              │              ├────────────>│             │
     │              │              │             │             │
```

## 5. Write Operation Flow (Independent Mode)

```
┌────────┐  ┌─────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐
│  App   │  │  Cache  │  │  Vector  │  │  Local   │  │   Gossip   │
│        │  │ Manager │  │  Clock   │  │ Storage  │  │   Engine   │
└───┬────┘  └────┬────┘  └────┬─────┘  └────┬─────┘  └─────┬──────┘
    │            │             │             │             │
    │Set(k,v)    │             │             │             │
    ├───────────>│             │             │             │
    │            │             │             │             │
    │            │ Increment(node_id)        │             │
    │            ├────────────>│             │             │
    │            │             │             │             │
    │            │ new_vclock  │             │             │
    │            │<────────────┤             │             │
    │            │             │             │             │
    │            │Set(k,v,vclock)            │             │
    │            ├─────────────┼────────────>│             │
    │            │             │             │             │
    │  OK        │             │             │             │
    │<───────────┤             │             │             │
    │            │             │             │             │
    │            │ Async: Broadcast DataUpdate (full data) │
    │            ├─────────────┼─────────────┼────────────>│
    │            │             │             │             │
    │            │             │             │             │ To Peers
    │            │             │             │             ├────────>
    │            │             │             │             │
```

## 6. Gossip Message Processing (Independent Mode)

```
┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│ Remote   │  │  Gossip  │  │  Vector  │  │ Conflict │  │  Local   │
│   Node   │  │  Engine  │  │  Clock   │  │ Resolver │  │ Storage  │
└────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │             │             │             │             │
     │ DataUpdate  │             │             │             │
     │ (k,v,vclock)│             │             │             │
     ├────────────>│             │             │             │
     │             │             │             │             │
     │             │ GetLocal(key)             │             │
     │             ├─────────────┼─────────────┼────────────>│
     │             │             │             │             │
     │             │ local_value, local_vclock │             │
     │             │<────────────┼─────────────┼─────────────┤
     │             │             │             │             │
     │             │ Compare(local_vclock, remote_vclock)    │
     │             ├────────────>│             │             │
     │             │             │             │             │
     │             │ Relation    │             │             │
     │             │<────────────┤             │             │
     │             │             │             │             │
     │             │ [If RemoteNewer: Update]  │             │
     │             │             │             │             │
     │             │ [If Concurrent: Conflict] │             │
     │             │             │             │             │
     │             │ Resolve(local, remote) ───>│             │
     │             │             │             │             │
     │             │             │             │ merged      │
     │             │<────────────┼─────────────┤             │
     │             │             │             │             │
     │             │Set(k,merged,merged_vclock)│             │
     │             ├─────────────┼─────────────┼────────────>│
     │             │             │             │             │
```

## 7. Anti-Entropy Synchronization

```
┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
│ Node A  │  │  Local  │  │  Gossip │  │ Node B  │  │  Local  │
│ (Init)  │  │ Storage │  │  Engine │  │ (Peer)  │  │ Storage │
└────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘
     │            │            │            │            │
     │[Periodic Trigger]       │            │            │
     │            │            │            │            │
     │GetDigest() │            │            │            │
     ├───────────>│            │            │            │
     │            │            │            │            │
     │merkle_root │            │            │            │
     │<───────────┤            │            │            │
     │            │            │            │            │
     │AntiEntropyReq(root)     │            │            │
     ├────────────┼───────────>│            │            │
     │            │            │            │            │
     │            │            │Forward Req │            │
     │            │            ├───────────>│            │
     │            │            │            │            │
     │            │            │            │GetDigest() │
     │            │            │            ├───────────>│
     │            │            │            │            │
     │            │            │            │merkle_root │
     │            │            │            │<───────────┤
     │            │            │            │            │
     │            │            │ Compare roots           │
     │            │            │            │            │
     │            │            │  [If Different]         │
     │            │            │            │            │
     │            │            │AntiEntropyResp          │
     │            │            │ (differing keys)        │
     │            │            │<───────────┤            │
     │            │            │            │            │
     │            │Process Differences      │            │
     │            │            │            │            │
     │            │ For each differing key: │            │
     │            │ - Compare versions      │            │
     │            │ - Pull from backing store (backed)  │
     │            │ - Or resolve conflict (independent) │
     │            │            │            │            │
     │UpdateLocal │            │            │            │
     │<───────────┤            │            │            │
     │            │            │            │            │
```

## 8. Node Join Process

```
┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│   New    │  │ Discovery│  │ Existing │  │  Gossip  │  │  Local   │
│   Node   │  │  Service │  │  Node    │  │  Engine  │  │ Storage  │
└────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │            │            │            │            │
     │Start()     │            │            │            │
     │            │            │            │            │
     │Discover()  │            │            │            │
     ├───────────>│            │            │            │
     │            │            │            │            │
     │peer_list   │            │            │            │
     │<───────────┤            │            │            │
     │            │            │            │            │
     │JoinRequest(node_info)   │            │            │
     ├────────────┼───────────>│            │            │
     │            │            │            │            │
     │            │            │Validate    │            │
     │            │            │            │            │
     │            │            │AddPeer()───>│            │
     │            │            │            │            │
     │JoinAck(peers, config)   │            │            │
     │<───────────┼────────────┤            │            │
     │            │            │            │            │
     │ConfigureGossip(config)  │            │            │
     ├────────────┼────────────┼───────────>│            │
     │            │            │            │            │
     │StartGossip()            │            │            │
     ├────────────┼────────────┼───────────>│            │
     │            │            │            │            │
     │            │            │            │Start       │
     │            │            │            │receiving   │
     │            │            │            │gossip      │
     │            │            │            │            │
     │TriggerAntiEntropy()     │            │            │
     ├────────────┼────────────┼───────────>│            │
     │            │            │            │            │
     │            │            │            │Sync with peers
     │            │            │            │            │
     │            │            │            │Populate────>│
     │            │            │            │cache       │
     │            │            │            │            │
     │Ready       │            │            │            │
     │            │            │            │            │
```

## 9. Failure Detection & Recovery

```
┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
│ Node A  │  │ Node B  │  │ Node C  │  │  Gossip │  │ Failure │
│(Monitor)│  │(Failed) │  │ (Peer)  │  │  Engine │  │Detector │
└────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘
     │            │            │            │            │
     │Gossip msg  │            │            │            │
     ├───────────>│            │            │            │
     │            │            │            │            │
     │ [Timeout - no response] │            │            │
     │            │            │            │            │
     │ReportTimeout(Node B)────────────────>│            │
     │            │            │            │            │
     │            │            │            │RecordFailure
     │            │            │            ├───────────>│
     │            │            │            │            │
     │            │            │            │            │[Count >= 3]
     │            │            │            │            │
     │            │            │            │MarkSuspect │
     │            │            │            │<───────────┤
     │            │            │            │            │
     │            │            │            │Broadcast   │
     │            │            │            │Suspect     │
     │NotifySuspect(Node B)    │            │            │
     │<───────────┼────────────┼────────────┤            │
     │            │            │            │            │
     │            │            │NotifySuspect(Node B)    │
     │            │            │<───────────┤            │
     │            │            │            │            │
     │            │[Node B recovers]        │            │
     │            │            │            │            │
     │            │AliveMsg────────────────>│            │
     │            │            │            │            │
     │            │            │            │ClearSuspect│
     │            │            │            ├───────────>│
     │            │            │            │            │
     │NotifyAlive(Node B)      │            │            │
     │<───────────┼────────────┼────────────┤            │
     │            │            │            │            │
     │TriggerSync(Node B)──────────────────>│            │
     │            │            │            │            │
     │            │            │            │Anti-entropy│
     │            │            │            │with Node B │
     │            │            │            │            │
```

## 10. Memory Management & Eviction

```
┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
│  Cache  │  │  Local  │  │Eviction │  │  Gossip │  │ Metrics │
│ Manager │  │ Storage │  │ Policy  │  │  Engine │  │Collector│
└────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘
     │            │            │            │            │
     │Set(k,v)    │            │            │            │
     ├───────────>│            │            │            │
     │            │            │            │            │
     │            │CheckSize() │            │            │
     │            │            │            │            │
     │            │[Size > MaxSize]         │            │
     │            │            │            │            │
     │            │SelectVictims()          │            │
     │            ├───────────>│            │            │
     │            │            │            │            │
     │            │            │[LRU/LFU/TTL]           │
     │            │            │Calculate   │            │
     │            │            │scores      │            │
     │            │            │            │            │
     │            │victim_keys │            │            │
     │            │<───────────┤            │            │
     │            │            │            │            │
     │            │Delete(keys)│            │            │
     │            │            │            │            │
     │            │RecordEviction()─────────┼───────────>│
     │            │            │            │            │
     │            │Insert(k,v) │            │            │
     │            │            │            │            │
     │            │            │            │[Optional]  │
     │            │            │            │Broadcast   │
     │            │            │            │Tombstone   │
     │            │            │            │(if delete) │
     │            │            │            │            │
     │  OK        │            │            │            │
     │<───────────┤            │            │            │
     │            │            │            │            │
     │            │[Background: Periodic TTL scan]       │
     │            │            │            │            │
     │            │ScanExpired()            │            │
     │            │            │            │            │
     │            │DeleteExpired()          │            │
     │            │            │            │            │
```

## 11. Health Check & Status Reporting

```
┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
│  HTTP   │  │  Cache  │  │  Gossip │  │ Backing │  │  Local  │
│   API   │  │ Manager │  │  Engine │  │  Store  │  │ Storage │
└────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘
     │            │            │            │            │
     │GET /health │            │            │            │
     ├───────────>│            │            │            │
     │            │            │            │            │
     │            │GetPeers()  │            │            │
     │            ├───────────>│            │            │
     │            │            │            │            │
     │            │peer_list   │            │            │
     │            │(with status)            │            │
     │            │<───────────┤            │            │
     │            │            │            │            │
     │            │Ping()──────┼───────────>│            │
     │            │            │            │            │
     │            │[If Backed Mode]         │            │
     │            │            │            │            │
     │            │            │            │PONG        │
     │            │<───────────┼────────────┤            │
     │            │            │            │            │
     │            │GetStats()  │            │            │
     │            ├────────────┼────────────┼───────────>│
     │            │            │            │            │
     │            │stats       │            │            │
     │            │<───────────┼────────────┼────────────┤
     │            │            │            │            │
     │            │BuildStatus()            │            │
     │            │            │            │            │
     │{          │            │            │            │
     │ status: OK│            │            │            │
     │ peers: 5  │            │            │            │
     │ backing: OK            │            │            │
     │ cache_size: 1.2GB      │            │            │
     │}           │            │            │            │
     │<───────────┤            │            │            │
     │            │            │            │            │
```

## Component Interaction Summary

### Key Interaction Patterns

1. **Read Path**:
   - App → Cache Manager → Local Storage (fast path)
   - Cache Manager → Backing Store (on miss, backed mode)

2. **Write Path**:
   - App → Cache Manager → Backing Store (if backed mode)
   - Cache Manager → Local Storage (update local)
   - Cache Manager → Gossip Engine (async broadcast)

3. **Gossip Path**:
   - Gossip Engine → Network Layer → Remote Nodes
   - Remote Nodes → Gossip Engine → Cache Manager
   - Cache Manager → Local Storage (update) or Backing Store (pull)

4. **Consistency Path**:
   - Periodic: Gossip Engine → Anti-Entropy
   - Anti-Entropy → Local Storage (digest)
   - Anti-Entropy → Remote Nodes (compare)
   - Anti-Entropy → Cache Manager (sync)

5. **Management Path**:
   - HTTP API → Cache Manager → All Components
   - Metrics Collector → All Components → Metrics Endpoint

### Component Dependencies

```
Cache Manager
├── Local Storage Engine (required)
├── Backing Store Connector (optional, backed mode)
├── Gossip Engine (required)
└── Metrics Collector (optional)

Gossip Engine
├── Network Layer (required)
├── Discovery Service (required)
└── Vector Clock Manager (independent mode)

Network Layer
└── Discovery Service (required)
```

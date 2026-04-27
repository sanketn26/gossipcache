# Phase 2: Backed Mode Implementation

**Goal**: Implement backed mode with Redis support, metadata gossip, and change detection/pull mechanism.

**Duration**: 3-4 weeks

**Prerequisites**: Phase 1 complete

**Status**: Not Started

## Overview

Phase 2 builds the distributed cache functionality for backed mode. We'll implement the backing store abstraction starting with Redis, create the gossip protocol for metadata propagation, and implement the pull-based data synchronization mechanism.

## Objectives

- [ ] Backing store abstraction and interface
- [ ] Redis connector with connection pooling
- [ ] Metadata gossip protocol implementation
- [ ] Change detection and pull mechanism
- [ ] Singleflight pattern for thundering herd prevention
- [ ] Network layer (TCP/UDP)
- [ ] Gossip engine with peer management
- [ ] Anti-entropy synchronization
- [ ] Node discovery (static peers for now)
- [ ] Multi-node integration tests
- [ ] Performance benchmarks for distributed scenarios

## Architecture Reference

Review these documents before starting:
- [Backed Mode Sequences](../diagrams/BACKED_MODE_SEQUENCES.md)
- [Architecture - Backed Mode](../ARCHITECTURE.md#backed-mode)
- [Technical Spec - Backing Store](../TECHNICAL_SPEC.md#42-backing-store-interface)

## Package Structure Updates

```
gossipcache/
├── internal/
│   ├── backingstore/
│   │   ├── backingstore.go        # Interface
│   │   ├── redis/
│   │   │   ├── redis.go           # Redis implementation
│   │   │   └── redis_test.go
│   │   └── mock/
│   │       └── mock_backingstore.go
│   ├── gossip/
│   │   ├── engine.go              # Gossip engine
│   │   ├── message.go             # Message types
│   │   ├── protocol.go            # Protocol logic
│   │   ├── peer.go                # Peer management
│   │   └── antientropy.go         # Anti-entropy
│   ├── network/
│   │   ├── transport.go           # TCP/UDP transport
│   │   ├── codec.go               # Message encoding/decoding
│   │   └── discovery.go           # Node discovery
│   ├── vclock/                    # (Phase 3)
│   └── util/
│       └── singleflight.go        # DRY: Shared singleflight
└── test/
    └── integration/
        ├── backed_mode_test.go
        └── multi_node_test.go
```

## Implementation Steps

### Step 1: Backing Store Interface (Day 1-2)

**SOLID**: Interface Segregation - Clean interface for backing stores

#### 1.1 Define Backing Store Interface

```go
// internal/backingstore/backingstore.go
package backingstore

import (
    "context"
    "time"
)

// BackingStore defines the interface for persistent storage backends
// ISP: Focused interface for backing store operations
type BackingStore interface {
    // Get retrieves a value and its version
    Get(ctx context.Context, key string) (value []byte, version int64, err error)

    // Set stores a value and returns the new version
    Set(ctx context.Context, key string, value []byte) (version int64, err error)

    // Delete removes a key
    Delete(ctx context.Context, key string) error

    // GetMulti retrieves multiple keys
    GetMulti(ctx context.Context, keys []string) (map[string]*Entry, error)

    // SetMulti stores multiple entries
    SetMulti(ctx context.Context, entries map[string][]byte) (map[string]int64, error)

    // Ping checks connectivity
    Ping(ctx context.Context) error

    // Close releases resources
    Close() error
}

// Entry represents a backing store entry
type Entry struct {
    Key     string
    Value   []byte
    Version int64
}

// Config holds backing store configuration
type Config struct {
    Type     string // "redis", "postgres", etc.
    Address  string
    Database string
    Username string
    Password string
    PoolSize int
    Timeout  time.Duration
}
```

### Step 2: Redis Connector (Day 2-5)

**SOLID**: Single Responsibility - Redis connector handles only Redis communication

#### 2.1 Redis Implementation

```go
// internal/backingstore/redis/redis.go
package redis

import (
    "context"
    "fmt"
    "strconv"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/sanketn26/gossipcache/internal/backingstore"
)

// RedisStore implements BackingStore using Redis
type RedisStore struct {
    client  *redis.Client
    timeout time.Duration
}

// New creates a new Redis backing store
func New(cfg *backingstore.Config) (*RedisStore, error) {
    opts := &redis.Options{
        Addr:         cfg.Address,
        Password:     cfg.Password,
        DB:           parseDB(cfg.Database),
        PoolSize:     cfg.PoolSize,
        DialTimeout:  cfg.Timeout,
        ReadTimeout:  cfg.Timeout,
        WriteTimeout: cfg.Timeout,
    }

    client := redis.NewClient(opts)

    // Test connection
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := client.Ping(ctx).Err(); err != nil {
        return nil, fmt.Errorf("redis ping failed: %w", err)
    }

    return &RedisStore{
        client:  client,
        timeout: cfg.Timeout,
    }, nil
}

func (r *RedisStore) Get(ctx context.Context, key string) ([]byte, int64, error) {
    // Use Redis hash to store value and version
    // HGETALL cache:{key} -> {value: ..., version: ...}

    hashKey := fmt.Sprintf("cache:%s", key)

    result, err := r.client.HGetAll(ctx, hashKey).Result()
    if err != nil {
        return nil, 0, err
    }

    if len(result) == 0 {
        return nil, 0, backingstore.ErrKeyNotFound
    }

    value := []byte(result["value"])
    version, _ := strconv.ParseInt(result["version"], 10, 64)

    return value, version, nil
}

func (r *RedisStore) Set(ctx context.Context, key string, value []byte) (int64, error) {
    hashKey := fmt.Sprintf("cache:%s", key)

    // Use Lua script for atomic version increment
    script := redis.NewScript(`
        local hashKey = KEYS[1]
        local value = ARGV[1]

        -- Get current version or start at 0
        local version = redis.call('HGET', hashKey, 'version')
        if not version then
            version = 0
        end
        version = tonumber(version) + 1

        -- Set value and version
        redis.call('HSET', hashKey, 'value', value, 'version', version)

        return version
    `)

    result, err := script.Run(ctx, r.client, []string{hashKey}, value).Result()
    if err != nil {
        return 0, err
    }

    version, ok := result.(int64)
    if !ok {
        return 0, fmt.Errorf("unexpected version type: %T", result)
    }

    return version, nil
}

func (r *RedisStore) Delete(ctx context.Context, key string) error {
    hashKey := fmt.Sprintf("cache:%s", key)
    return r.client.Del(ctx, hashKey).Err()
}

func (r *RedisStore) GetMulti(ctx context.Context, keys []string) (map[string]*backingstore.Entry, error) {
    result := make(map[string]*backingstore.Entry)

    // Use pipeline for efficiency
    pipe := r.client.Pipeline()
    cmds := make(map[string]*redis.MapStringStringCmd)

    for _, key := range keys {
        hashKey := fmt.Sprintf("cache:%s", key)
        cmds[key] = pipe.HGetAll(ctx, hashKey)
    }

    if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
        return nil, err
    }

    for key, cmd := range cmds {
        data, err := cmd.Result()
        if err != nil || len(data) == 0 {
            continue
        }

        version, _ := strconv.ParseInt(data["version"], 10, 64)
        result[key] = &backingstore.Entry{
            Key:     key,
            Value:   []byte(data["value"]),
            Version: version,
        }
    }

    return result, nil
}

func (r *RedisStore) SetMulti(ctx context.Context, entries map[string][]byte) (map[string]int64, error) {
    result := make(map[string]int64)

    // Use pipeline for efficiency
    pipe := r.client.Pipeline()

    for key, value := range entries {
        hashKey := fmt.Sprintf("cache:%s", key)

        // Simplified: just increment version
        pipe.HIncrBy(ctx, hashKey, "version", 1)
        pipe.HSet(ctx, hashKey, "value", value)
    }

    if _, err := pipe.Exec(ctx); err != nil {
        return nil, err
    }

    // For simplicity, fetch versions after
    for key := range entries {
        hashKey := fmt.Sprintf("cache:%s", key)
        version, _ := r.client.HGet(ctx, hashKey, "version").Int64()
        result[key] = version
    }

    return result, nil
}

func (r *RedisStore) Ping(ctx context.Context) error {
    return r.client.Ping(ctx).Err()
}

func (r *RedisStore) Close() error {
    return r.client.Close()
}

func parseDB(db string) int {
    parsed, _ := strconv.Atoi(db)
    return parsed
}
```

#### 2.2 Redis Tests

```go
// internal/backingstore/redis/redis_test.go
package redis

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/sanketn26/gossipcache/internal/backingstore"
)

func TestRedisStore_GetSet(t *testing.T) {
    // Skip if no Redis available
    if testing.Short() {
        t.Skip("Skipping Redis integration test")
    }

    store, err := New(&backingstore.Config{
        Address:  "localhost:6379",
        Database: "0",
        PoolSize: 10,
        Timeout:  5 * time.Second,
    })
    require.NoError(t, err)
    defer store.Close()

    ctx := context.Background()

    // Test Set
    version1, err := store.Set(ctx, "test_key", []byte("test_value"))
    require.NoError(t, err)
    require.Greater(t, version1, int64(0))

    // Test Get
    value, version2, err := store.Get(ctx, "test_key")
    require.NoError(t, err)
    require.Equal(t, []byte("test_value"), value)
    require.Equal(t, version1, version2)

    // Test Update increments version
    version3, err := store.Set(ctx, "test_key", []byte("updated_value"))
    require.NoError(t, err)
    require.Greater(t, version3, version1)

    // Cleanup
    store.Delete(ctx, "test_key")
}
```

### Step 3: Gossip Message Types (Day 5-6)

**SOLID**: Open/Closed - Message types can be extended without modifying core

#### 3.1 Message Definitions

```go
// internal/gossip/message.go
package gossip

import (
    "encoding/binary"
    "fmt"
    "time"
)

// MessageType represents the type of gossip message
type MessageType uint16

const (
    MsgChangeNotification MessageType = 1
    MsgAntiEntropyReq     MessageType = 3
    MsgAntiEntropyResp    MessageType = 4
    MsgJoinRequest        MessageType = 5
    MsgJoinAck            MessageType = 6
    MsgPing               MessageType = 9
    MsgAck                MessageType = 10
)

// Message is the interface for all gossip messages
type Message interface {
    Type() MessageType
    Serialize() ([]byte, error)
}

// ChangeNotification notifies peers of a key change (backed mode)
type ChangeNotification struct {
    Key       string
    Version   int64
    Checksum  string    // SHA256 of value
    Timestamp time.Time
    NodeID    string
}

func (m *ChangeNotification) Type() MessageType {
    return MsgChangeNotification
}

func (m *ChangeNotification) Serialize() ([]byte, error) {
    // Simple binary encoding (could use protobuf/msgpack in production)
    // Format: [key_len][key][version][checksum_len][checksum][timestamp][node_id_len][node_id]

    buf := make([]byte, 0, 256)

    // Key
    keyLen := uint16(len(m.Key))
    buf = binary.BigEndian.AppendUint16(buf, keyLen)
    buf = append(buf, []byte(m.Key)...)

    // Version
    buf = binary.BigEndian.AppendUint64(buf, uint64(m.Version))

    // Checksum
    checksumLen := uint16(len(m.Checksum))
    buf = binary.BigEndian.AppendUint16(buf, checksumLen)
    buf = append(buf, []byte(m.Checksum)...)

    // Timestamp
    buf = binary.BigEndian.AppendUint64(buf, uint64(m.Timestamp.Unix()))

    // NodeID
    nodeIDLen := uint16(len(m.NodeID))
    buf = binary.BigEndian.AppendUint16(buf, nodeIDLen)
    buf = append(buf, []byte(m.NodeID)...)

    return buf, nil
}

func DeserializeChangeNotification(data []byte) (*ChangeNotification, error) {
    if len(data) < 2 {
        return nil, fmt.Errorf("data too short")
    }

    msg := &ChangeNotification{}
    offset := 0

    // Key
    keyLen := binary.BigEndian.Uint16(data[offset:])
    offset += 2
    msg.Key = string(data[offset : offset+int(keyLen)])
    offset += int(keyLen)

    // Version
    msg.Version = int64(binary.BigEndian.Uint64(data[offset:]))
    offset += 8

    // Checksum
    checksumLen := binary.BigEndian.Uint16(data[offset:])
    offset += 2
    msg.Checksum = string(data[offset : offset+int(checksumLen)])
    offset += int(checksumLen)

    // Timestamp
    timestamp := int64(binary.BigEndian.Uint64(data[offset:]))
    msg.Timestamp = time.Unix(timestamp, 0)
    offset += 8

    // NodeID
    nodeIDLen := binary.BigEndian.Uint16(data[offset:])
    offset += 2
    msg.NodeID = string(data[offset : offset+int(nodeIDLen)])

    return msg, nil
}

// JoinRequest is sent by new nodes joining the cluster
type JoinRequest struct {
    NodeID    string
    Address   string
    Timestamp time.Time
}

func (m *JoinRequest) Type() MessageType {
    return MsgJoinRequest
}

// JoinAck is the response to a join request
type JoinAck struct {
    ClusterID string
    Peers     []PeerInfo
}

func (m *JoinAck) Type() MessageType {
    return MsgJoinAck
}

type PeerInfo struct {
    NodeID   string
    Address  string
    LastSeen time.Time
}
```

### Step 4: Network Layer (Day 6-8)

**SOLID**: Single Responsibility - Transport handles only network I/O

#### 4.1 TCP/UDP Transport

```go
// internal/network/transport.go
package network

import (
    "context"
    "encoding/binary"
    "fmt"
    "net"
    "sync"

    "github.com/sanketn26/gossipcache/internal/gossip"
)

const (
    MagicNumber = 0x47534350 // "GSCP"
    Version     = 1
)

// Transport handles network communication
// SRP: Responsible only for sending/receiving messages
type Transport struct {
    tcpListener net.Listener
    udpConn     *net.UDPConn

    handlers map[gossip.MessageType]MessageHandler
    mu       sync.RWMutex

    closed   chan struct{}
    wg       sync.WaitGroup
}

type MessageHandler func(msg gossip.Message, from net.Addr) error

func NewTransport(tcpPort, udpPort int) (*Transport, error) {
    // TCP listener
    tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", tcpPort))
    if err != nil {
        return nil, fmt.Errorf("tcp listen: %w", err)
    }

    // UDP connection
    udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", udpPort))
    if err != nil {
        tcpListener.Close()
        return nil, fmt.Errorf("resolve udp addr: %w", err)
    }

    udpConn, err := net.ListenUDP("udp", udpAddr)
    if err != nil {
        tcpListener.Close()
        return nil, fmt.Errorf("udp listen: %w", err)
    }

    t := &Transport{
        tcpListener: tcpListener,
        udpConn:     udpConn,
        handlers:    make(map[gossip.MessageType]MessageHandler),
        closed:      make(chan struct{}),
    }

    // Start listeners
    t.wg.Add(2)
    go t.listenTCP()
    go t.listenUDP()

    return t, nil
}

func (t *Transport) RegisterHandler(msgType gossip.MessageType, handler MessageHandler) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.handlers[msgType] = handler
}

func (t *Transport) SendTCP(ctx context.Context, addr string, msg gossip.Message) error {
    conn, err := net.Dial("tcp", addr)
    if err != nil {
        return err
    }
    defer conn.Close()

    return t.writeMessage(conn, msg)
}

func (t *Transport) SendUDP(ctx context.Context, addr string, msg gossip.Message) error {
    udpAddr, err := net.ResolveUDPAddr("udp", addr)
    if err != nil {
        return err
    }

    data, err := t.encodeMessage(msg)
    if err != nil {
        return err
    }

    _, err = t.udpConn.WriteToUDP(data, udpAddr)
    return err
}

func (t *Transport) listenTCP() {
    defer t.wg.Done()

    for {
        select {
        case <-t.closed:
            return
        default:
        }

        conn, err := t.tcpListener.Accept()
        if err != nil {
            continue
        }

        go t.handleTCPConn(conn)
    }
}

func (t *Transport) handleTCPConn(conn net.Conn) {
    defer conn.Close()

    msg, err := t.readMessage(conn)
    if err != nil {
        return
    }

    t.dispatchMessage(msg, conn.RemoteAddr())
}

func (t *Transport) listenUDP() {
    defer t.wg.Done()

    buf := make([]byte, 65536)

    for {
        select {
        case <-t.closed:
            return
        default:
        }

        n, addr, err := t.udpConn.ReadFromUDP(buf)
        if err != nil {
            continue
        }

        msg, err := t.decodeMessage(buf[:n])
        if err != nil {
            continue
        }

        t.dispatchMessage(msg, addr)
    }
}

func (t *Transport) dispatchMessage(msg gossip.Message, from net.Addr) {
    t.mu.RLock()
    handler, ok := t.handlers[msg.Type()]
    t.mu.RUnlock()

    if ok && handler != nil {
        handler(msg, from)
    }
}

func (t *Transport) writeMessage(conn net.Conn, msg gossip.Message) error {
    data, err := t.encodeMessage(msg)
    if err != nil {
        return err
    }

    _, err = conn.Write(data)
    return err
}

func (t *Transport) readMessage(conn net.Conn) (gossip.Message, error) {
    // Read header (magic + version + type + length)
    header := make([]byte, 12)
    if _, err := conn.Read(header); err != nil {
        return nil, err
    }

    // Verify magic number
    magic := binary.BigEndian.Uint32(header[0:4])
    if magic != MagicNumber {
        return nil, fmt.Errorf("invalid magic number")
    }

    // Read message type and length
    msgType := gossip.MessageType(binary.BigEndian.Uint16(header[6:8]))
    length := binary.BigEndian.Uint32(header[8:12])

    // Read payload
    payload := make([]byte, length)
    if _, err := conn.Read(payload); err != nil {
        return nil, err
    }

    return t.deserializeMessage(msgType, payload)
}

func (t *Transport) encodeMessage(msg gossip.Message) ([]byte, error) {
    payload, err := msg.Serialize()
    if err != nil {
        return nil, err
    }

    // Build message: [magic][version][type][length][payload]
    buf := make([]byte, 12+len(payload))

    binary.BigEndian.PutUint32(buf[0:4], MagicNumber)
    binary.BigEndian.PutUint16(buf[4:6], Version)
    binary.BigEndian.PutUint16(buf[6:8], uint16(msg.Type()))
    binary.BigEndian.PutUint32(buf[8:12], uint32(len(payload)))
    copy(buf[12:], payload)

    return buf, nil
}

func (t *Transport) decodeMessage(data []byte) (gossip.Message, error) {
    if len(data) < 12 {
        return nil, fmt.Errorf("message too short")
    }

    msgType := gossip.MessageType(binary.BigEndian.Uint16(data[6:8]))
    payload := data[12:]

    return t.deserializeMessage(msgType, payload)
}

func (t *Transport) deserializeMessage(msgType gossip.MessageType, payload []byte) (gossip.Message, error) {
    switch msgType {
    case gossip.MsgChangeNotification:
        return gossip.DeserializeChangeNotification(payload)
    // Add other message types...
    default:
        return nil, fmt.Errorf("unknown message type: %d", msgType)
    }
}

func (t *Transport) Close() error {
    close(t.closed)

    t.tcpListener.Close()
    t.udpConn.Close()

    t.wg.Wait()
    return nil
}
```

### Step 4.5: Connection Pooling & Backpressure (Day 8-9)

**SOLID**: Single Responsibility - Connection manager handles only connection lifecycle

#### 4.5.1 TCP Connection Pool

```go
// internal/network/pool.go
package network

import (
    "context"
    "errors"
    "net"
    "sync"
    "time"
)

// ConnectionPool manages TCP connections to peers
type ConnectionPool struct {
    pools map[string]*peerPool
    mu    sync.RWMutex

    maxConnsPerPeer int
    connTimeout     time.Duration
}

type peerPool struct {
    addr  string
    conns chan net.Conn
    mu    sync.Mutex
}

func NewConnectionPool(maxConnsPerPeer int, connTimeout time.Duration) *ConnectionPool {
    return &ConnectionPool{
        pools:           make(map[string]*peerPool),
        maxConnsPerPeer: maxConnsPerPeer,
        connTimeout:     connTimeout,
    }
}

func (cp *ConnectionPool) Get(ctx context.Context, addr string) (net.Conn, error) {
    pool := cp.getOrCreatePool(addr)

    // Try to get existing connection
    select {
    case conn := <-pool.conns:
        // Check if connection is still alive
        if err := checkConn(conn); err == nil {
            return conn, nil
        }
        conn.Close()
    default:
    }

    // Create new connection
    dialer := &net.Dialer{Timeout: cp.connTimeout}
    return dialer.DialContext(ctx, "tcp", addr)
}

func (cp *ConnectionPool) Put(addr string, conn net.Conn) {
    pool := cp.getOrCreatePool(addr)

    select {
    case pool.conns <- conn:
        // Connection returned to pool
    default:
        // Pool full, close connection
        conn.Close()
    }
}

func (cp *ConnectionPool) getOrCreatePool(addr string) *peerPool {
    cp.mu.RLock()
    pool, exists := cp.pools[addr]
    cp.mu.RUnlock()

    if exists {
        return pool
    }

    cp.mu.Lock()
    defer cp.mu.Unlock()

    // Double check
    if pool, exists = cp.pools[addr]; exists {
        return pool
    }

    pool = &peerPool{
        addr:  addr,
        conns: make(chan net.Conn, cp.maxConnsPerPeer),
    }
    cp.pools[addr] = pool

    return pool
}

func checkConn(conn net.Conn) error {
    // Set short deadline for check
    conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
    defer conn.SetReadDeadline(time.Time{})

    one := make([]byte, 1)
    _, err := conn.Read(one)

    if err == nil {
        // Put byte back (shouldn't happen for healthy conn)
        return errors.New("unexpected data")
    }

    // Check for timeout (connection is alive)
    if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
        return nil
    }

    return err
}

func (cp *ConnectionPool) Close() error {
    cp.mu.Lock()
    defer cp.mu.Unlock()

    for _, pool := range cp.pools {
        close(pool.conns)
        for conn := range pool.conns {
            conn.Close()
        }
    }

    return nil
}
```

#### 4.5.2 Update Transport with Connection Pool

```go
// internal/network/transport.go (updated)
func NewTransport(tcpPort, udpPort int) (*Transport, error) {
    // ... existing code ...

    t := &Transport{
        tcpListener: tcpListener,
        udpConn:     udpConn,
        handlers:    make(map[gossip.MessageType]MessageHandler),
        connPool:    NewConnectionPool(10, 5*time.Second), // Add pool
        closed:      make(chan struct{}),
    }

    return t, nil
}

func (t *Transport) SendTCP(ctx context.Context, addr string, msg gossip.Message) error {
    // Get connection from pool
    conn, err := t.connPool.Get(ctx, addr)
    if err != nil {
        return err
    }

    // Try to send message
    err = t.writeMessage(conn, msg)

    if err != nil {
        // Connection failed, close it
        conn.Close()
        return err
    }

    // Return connection to pool
    t.connPool.Put(addr, conn)
    return nil
}
```

#### 4.5.3 Backpressure Handling

```go
// internal/gossip/queue.go
package gossip

import (
    "context"
    "errors"
    "sync"
)

var ErrQueueFull = errors.New("gossip queue full")

// GossipQueue provides backpressure for gossip messages
type GossipQueue struct {
    messages chan *QueuedMessage
    maxSize  int
    dropped  int64 // atomic counter
    mu       sync.RWMutex
}

type QueuedMessage struct {
    Message gossip.Message
    Target  string
    Retries int
}

func NewGossipQueue(maxSize int) *GossipQueue {
    return &GossipQueue{
        messages: make(chan *QueuedMessage, maxSize),
        maxSize:  maxSize,
    }
}

func (gq *GossipQueue) Enqueue(ctx context.Context, msg *QueuedMessage) error {
    select {
    case gq.messages <- msg:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Queue full - apply backpressure
        atomic.AddInt64(&gq.dropped, 1)
        return ErrQueueFull
    }
}

func (gq *GossipQueue) Dequeue(ctx context.Context) (*QueuedMessage, error) {
    select {
    case msg := <-gq.messages:
        return msg, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (gq *GossipQueue) Len() int {
    return len(gq.messages)
}

func (gq *GossipQueue) Dropped() int64 {
    return atomic.LoadInt64(&gq.dropped)
}
```

#### 4.5.4 Integrate Queue with Gossip Engine

```go
// internal/gossip/engine.go (updated)
type Engine struct {
    nodeID  string

    transport   *network.Transport
    storage     storage.Storage
    backingStore backingstore.BackingStore
    peers       *PeerManager

    queue       *GossipQueue  // Add queue
    config      *Config
    closed      chan struct{}
    wg          sync.WaitGroup
}

func NewEngine(...) *Engine {
    e := &Engine{
        // ... existing fields ...
        queue: NewGossipQueue(1000), // Add queue
    }

    // Start queue processor
    e.wg.Add(1)
    go e.processQueue()

    return e
}

func (e *Engine) BroadcastChange(ctx context.Context, key string, version int64, value []byte) error {
    checksum := fmt.Sprintf("%x", sha256.Sum256(value))

    msg := &ChangeNotification{
        Key:       key,
        Version:   version,
        Checksum:  checksum,
        Timestamp: time.Now(),
        NodeID:    e.nodeID,
    }

    // Enqueue messages for async processing
    peers := e.selectGossipPeers()

    for _, peer := range peers {
        queuedMsg := &QueuedMessage{
            Message: msg,
            Target:  peer.Address,
            Retries: 0,
        }

        // Non-blocking enqueue with backpressure
        if err := e.queue.Enqueue(ctx, queuedMsg); err != nil {
            // Log dropped message
            logger.Warn("gossip queue full, message dropped",
                "peer", peer.Address,
                "dropped_total", e.queue.Dropped())
        }
    }

    return nil
}

func (e *Engine) processQueue() {
    defer e.wg.Done()

    for {
        select {
        case <-e.closed:
            return
        default:
        }

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        msg, err := e.queue.Dequeue(ctx)
        cancel()

        if err != nil {
            continue
        }

        // Send message
        if err := e.transport.SendUDP(context.Background(), msg.Target, msg.Message); err != nil {
            // Retry logic
            if msg.Retries < 3 {
                msg.Retries++
                e.queue.Enqueue(context.Background(), msg)
            }
        }
    }
}
```

#### 4.5.5 Testing Connection Pool and Backpressure

```go
// internal/network/pool_test.go
func TestConnectionPool_ReuseConnection(t *testing.T) {
    pool := NewConnectionPool(5, 5*time.Second)
    defer pool.Close()

    // Start test server
    listener := startTestServer(t)
    defer listener.Close()

    ctx := context.Background()

    // Get connection
    conn1, err := pool.Get(ctx, listener.Addr().String())
    require.NoError(t, err)

    // Return to pool
    pool.Put(listener.Addr().String(), conn1)

    // Get again - should reuse
    conn2, err := pool.Get(ctx, listener.Addr().String())
    require.NoError(t, err)

    // Should be same connection (can verify with conn.LocalAddr())
    assert.Equal(t, conn1.LocalAddr(), conn2.LocalAddr())
}

// internal/gossip/queue_test.go
func TestGossipQueue_Backpressure(t *testing.T) {
    queue := NewGossipQueue(10) // Small queue

    ctx := context.Background()

    // Fill queue
    for i := 0; i < 10; i++ {
        msg := &QueuedMessage{Target: fmt.Sprintf("peer%d", i)}
        err := queue.Enqueue(ctx, msg)
        require.NoError(t, err)
    }

    // Next message should trigger backpressure
    msg := &QueuedMessage{Target: "peer11"}
    err := queue.Enqueue(ctx, msg)
    assert.ErrorIs(t, err, ErrQueueFull)
    assert.Equal(t, int64(1), queue.Dropped())
}
```

**Benefits**:
- **Connection Pooling**: Reduces connection overhead, improves throughput
- **Backpressure**: Prevents memory exhaustion, graceful degradation
- **Observability**: Track dropped messages, queue depth

### Step 5: Gossip Engine (Day 9-13)

**SOLID**: Single Responsibility - Gossip engine coordinates gossip protocol

#### 5.1 Peer Management

```go
// internal/gossip/peer.go
package gossip

import (
    "sync"
    "time"
)

// PeerManager tracks cluster peers
type PeerManager struct {
    mu    sync.RWMutex
    peers map[string]*Peer
}

type Peer struct {
    NodeID   string
    Address  string
    LastSeen time.Time
    Status   PeerStatus
}

type PeerStatus int

const (
    StatusAlive PeerStatus = iota
    StatusSuspected
    StatusDead
)

func NewPeerManager() *PeerManager {
    return &PeerManager{
        peers: make(map[string]*Peer),
    }
}

func (pm *PeerManager) AddPeer(nodeID, address string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    pm.peers[nodeID] = &Peer{
        NodeID:   nodeID,
        Address:  address,
        LastSeen: time.Now(),
        Status:   StatusAlive,
    }
}

func (pm *PeerManager) GetPeers() []*Peer {
    pm.mu.RLock()
    defer pm.mu.RUnlock()

    peers := make([]*Peer, 0, len(pm.peers))
    for _, peer := range pm.peers {
        peers = append(peers, peer)
    }

    return peers
}

func (pm *PeerManager) UpdateLastSeen(nodeID string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    if peer, ok := pm.peers[nodeID]; ok {
        peer.LastSeen = time.Now()
        peer.Status = StatusAlive
    }
}

func (pm *PeerManager) GetAlivePeers() []*Peer {
    pm.mu.RLock()
    defer pm.mu.RUnlock()

    peers := make([]*Peer, 0)
    for _, peer := range pm.peers {
        if peer.Status == StatusAlive {
            peers = append(peers, peer)
        }
    }

    return peers
}
```

#### 5.2 Gossip Engine Implementation

```go
// internal/gossip/engine.go
package gossip

import (
    "context"
    "crypto/sha256"
    "fmt"
    "math/rand"
    "sync"
    "time"

    "github.com/sanketn26/gossipcache/internal/backingstore"
    "github.com/sanketn26/gossipcache/internal/network"
    "github.com/sanketn26/gossipcache/internal/storage"
)

// Engine implements the gossip protocol
// SRP: Coordinates gossip rounds, change propagation, anti-entropy
type Engine struct {
    nodeID  string

    transport   *network.Transport
    storage     storage.Storage
    backingStore backingstore.BackingStore
    peers       *PeerManager

    config      *Config
    closed      chan struct{}
    wg          sync.WaitGroup
}

type Config struct {
    Interval            time.Duration
    Fanout              int
    AntiEntropyInterval time.Duration
}

func NewEngine(
    nodeID string,
    transport *network.Transport,
    storage storage.Storage,
    backingStore backingstore.BackingStore,
    config *Config,
) *Engine {
    e := &Engine{
        nodeID:       nodeID,
        transport:    transport,
        storage:      storage,
        backingStore: backingStore,
        peers:        NewPeerManager(),
        config:       config,
        closed:       make(chan struct{}),
    }

    // Register message handlers
    transport.RegisterHandler(MsgChangeNotification, e.handleChangeNotification)
    transport.RegisterHandler(MsgJoinRequest, e.handleJoinRequest)

    return e
}

func (e *Engine) Start(ctx context.Context) error {
    // Start gossip loop
    e.wg.Add(1)
    go e.gossipLoop()

    // Start anti-entropy loop
    e.wg.Add(1)
    go e.antiEntropyLoop()

    return nil
}

func (e *Engine) Stop() error {
    close(e.closed)
    e.wg.Wait()
    return nil
}

func (e *Engine) AddPeer(nodeID, address string) {
    e.peers.AddPeer(nodeID, address)
}

// BroadcastChange sends change notification to peers (backed mode)
func (e *Engine) BroadcastChange(ctx context.Context, key string, version int64, value []byte) error {
    // Calculate checksum
    checksum := fmt.Sprintf("%x", sha256.Sum256(value))

    msg := &ChangeNotification{
        Key:       key,
        Version:   version,
        Checksum:  checksum,
        Timestamp: time.Now(),
        NodeID:    e.nodeID,
    }

    // Select random peers
    peers := e.selectGossipPeers()

    // Send to each peer (async, best-effort)
    for _, peer := range peers {
        go func(p *Peer) {
            ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
            defer cancel()

            e.transport.SendUDP(ctx, p.Address, msg)
        }(peer)
    }

    return nil
}

func (e *Engine) handleChangeNotification(msg Message, from net.Addr) error {
    notification, ok := msg.(*ChangeNotification)
    if !ok {
        return fmt.Errorf("invalid message type")
    }

    ctx := context.Background()

    // Get local entry
    localEntry, err := e.storage.Get(ctx, notification.Key)

    // If not in local cache or version is newer, pull from backing store
    if err != nil || localEntry == nil || e.needsPull(localEntry, notification) {
        return e.pullFromBackingStore(ctx, notification)
    }

    return nil
}

func (e *Engine) needsPull(localEntry *storage.Entry, notification *ChangeNotification) bool {
    // For backed mode, we could store version in metadata
    // For simplicity, always pull if checksum differs
    localChecksum := fmt.Sprintf("%x", sha256.Sum256(localEntry.Value))
    return localChecksum != notification.Checksum
}

func (e *Engine) pullFromBackingStore(ctx context.Context, notification *ChangeNotification) error {
    // Pull from backing store
    value, version, err := e.backingStore.Get(ctx, notification.Key)
    if err != nil {
        return fmt.Errorf("pull from backing store: %w", err)
    }

    // Verify version
    if version < notification.Version {
        // Backing store is stale, this shouldn't happen
        return fmt.Errorf("backing store version mismatch")
    }

    // Update local cache
    // We need to extend storage to support version metadata
    // For now, use TTL-based approach
    return e.storage.Set(ctx, notification.Key, value, 5*time.Minute)
}

func (e *Engine) selectGossipPeers() []*Peer {
    alivePeers := e.peers.GetAlivePeers()

    if len(alivePeers) <= e.config.Fanout {
        return alivePeers
    }

    // Select random subset
    rand.Shuffle(len(alivePeers), func(i, j int) {
        alivePeers[i], alivePeers[j] = alivePeers[j], alivePeers[i]
    })

    return alivePeers[:e.config.Fanout]
}

func (e *Engine) gossipLoop() {
    defer e.wg.Done()

    ticker := time.NewTicker(e.config.Interval)
    defer ticker.Stop()

    for {
        select {
        case <-e.closed:
            return
        case <-ticker.C:
            // Gossip round happens on-demand when changes occur
            // This loop can be used for heartbeats/failure detection
        }
    }
}

func (e *Engine) antiEntropyLoop() {
    defer e.wg.Done()

    ticker := time.NewTicker(e.config.AntiEntropyInterval)
    defer ticker.Stop()

    for {
        select {
        case <-e.closed:
            return
        case <-ticker.C:
            e.performAntiEntropy()
        }
    }
}

func (e *Engine) performAntiEntropy() {
    peers := e.selectGossipPeers()
    if len(peers) == 0 {
        return
    }

    // Select random peer for anti-entropy
    peer := peers[rand.Intn(len(peers))]

    // Anti-entropy implementation (simplified)
    // In production: exchange merkle trees, identify differences, sync
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    tree, err := e.buildMerkleTree(ctx)
    if err != nil {
        logger.Warn("failed to build merkle tree", "error", err)
        return
    }

    req := &AntiEntropyRequest{
        NodeID:     e.nodeID,
        MerkleRoot: tree.RootHash(),
        KeyCount:   tree.KeyCount(),
        RequestID:  e.newRequestID(),
    }

    if err := e.transport.SendTCP(ctx, peer.Address, req); err != nil {
        logger.Warn("anti-entropy request failed", "peer", peer.NodeID, "error", err)
    }
}

func (e *Engine) handleJoinRequest(msg Message, from net.Addr) error {
    // Handle node joining cluster
    joinReq, ok := msg.(*JoinRequest)
    if !ok {
        return fmt.Errorf("invalid message type")
    }

    // Add to peers
    e.peers.AddPeer(joinReq.NodeID, joinReq.Address)

    ack := &JoinAck{
        ClusterID:    e.config.ClusterID,
        Peers:        e.peers.GetAliveNodeInfo(),
        GossipConfig: e.config.PublicGossipConfig(),
    }

    return e.transport.SendTCP(context.Background(), joinReq.Address, ack)
}
```

#### 5.5 Merkle Tree Comparison

```go
// internal/gossip/merkle.go
package gossip

import (
    "crypto/sha256"
    "fmt"
    "sort"
)

type MerkleTree struct {
    leaves []MerkleLeaf
    root   []byte
}

type MerkleLeaf struct {
    Key     string
    Version int64
    Hash    []byte
}

func BuildMerkleTree(entries []MerkleLeaf) *MerkleTree {
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Key < entries[j].Key
    })

    hashes := make([][]byte, 0, len(entries))
    for _, entry := range entries {
        sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", entry.Key, entry.Version)))
        hashes = append(hashes, sum[:])
    }

    return &MerkleTree{
        leaves: entries,
        root:   buildMerkleRoot(hashes),
    }
}

func (m *MerkleTree) RootHash() []byte {
    return append([]byte(nil), m.root...)
}

func (m *MerkleTree) KeyCount() int {
    return len(m.leaves)
}
```

Anti-entropy handling:
- If Merkle roots match, return success without transferring keys.
- If roots differ, exchange subtree hashes to narrow the differing key ranges.
- Pull differing keys from the backing store in backed mode, then update local storage.
- Track anti-entropy request latency, differing-key count, and repair errors in metrics.

### Step 6: Integrate with Cache Manager (Day 14-16)

**SOLID**: Dependency Inversion - Cache manager depends on abstractions

```go
// internal/cache/backed_cache.go
package cache

import (
    "context"
    "time"

    "github.com/sanketn26/gossipcache/internal/backingstore"
    "github.com/sanketn26/gossipcache/internal/gossip"
    "github.com/sanketn26/gossipcache/internal/storage"
    "github.com/sanketn26/gossipcache/internal/util"
)

// BackedCache implements Cache with backing store support
type BackedCache struct {
    storage      storage.Storage
    backingStore backingstore.BackingStore
    gossip       *gossip.Engine
    sf           *util.SingleFlight // DRY: Reuse singleflight
    config       *CacheConfig
}

func NewBackedCache(
    storage storage.Storage,
    backingStore backingstore.BackingStore,
    gossip *gossip.Engine,
    config *CacheConfig,
) *BackedCache {
    return &BackedCache{
        storage:      storage,
        backingStore: backingStore,
        gossip:       gossip,
        sf:           util.NewSingleFlight(),
        config:       config,
    }
}

func (c *BackedCache) Get(ctx context.Context, key string) ([]byte, error) {
    // Try local cache first
    entry, err := c.storage.Get(ctx, key)
    if err == nil {
        return entry.Value, nil
    }

    // Cache miss: pull from backing store (with singleflight)
    result := c.sf.Do(key, func() (interface{}, error) {
        return c.pullFromBackingStore(ctx, key)
    })

    if result.Err != nil {
        return nil, result.Err
    }

    return result.Val.([]byte), nil
}

func (c *BackedCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    // Write to backing store
    version, err := c.backingStore.Set(ctx, key, value)
    if err != nil {
        return err
    }

    // Update local cache
    if err := c.storage.Set(ctx, key, value, ttl); err != nil {
        return err
    }

    // Gossip change notification (async)
    go c.gossip.BroadcastChange(context.Background(), key, version, value)

    return nil
}

func (c *BackedCache) pullFromBackingStore(ctx context.Context, key string) ([]byte, error) {
    value, _, err := c.backingStore.Get(ctx, key)
    if err != nil {
        return nil, err
    }

    // Update local cache
    c.storage.Set(ctx, key, value, c.config.DefaultTTL)

    return value, nil
}
```

### Step 7: Integration Tests (Day 17-20)

```go
// test/integration/backed_mode_test.go
package integration

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

func TestBackedMode_MultiNode(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Setup 3-node cluster
    nodes := setupThreeNodeCluster(t)
    defer cleanupCluster(nodes)

    ctx := context.Background()

    // Write on node 1
    err := nodes[0].Set(ctx, "test_key", []byte("test_value"), 5*time.Minute)
    require.NoError(t, err)

    // Wait for gossip propagation
    time.Sleep(2 * time.Second)

    // Read from node 2 (should pull from Redis)
    value, err := nodes[1].Get(ctx, "test_key")
    require.NoError(t, err)
    require.Equal(t, []byte("test_value"), value)

    // Read from node 3 (should hit local cache now)
    value, err = nodes[2].Get(ctx, "test_key")
    require.NoError(t, err)
    require.Equal(t, []byte("test_value"), value)
}
```

## Deliverables

- [ ] Backing store interface and Redis implementation
- [ ] Gossip protocol with metadata propagation
- [ ] Network layer (TCP/UDP)
- [ ] Peer management and discovery
- [ ] Change detection and pull mechanism
- [ ] Singleflight pattern implemented
- [ ] Multi-node integration tests
- [ ] Performance benchmarks

## Success Criteria

1. **Functional**: 3-node cluster with backed mode working
2. **Performance**: Gossip propagation < 500ms
3. **Quality**: >80% test coverage, integration tests passing
4. **Scalability**: Tested with 5-10 nodes

## Next Phase

Once Phase 2 is complete, move to [Phase 3: Independent Mode](PHASE_3_INDEPENDENT_MODE.md) to add vector clocks and full-data gossip.

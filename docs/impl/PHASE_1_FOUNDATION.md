# Phase 1: Core Foundation

**Goal**: Build the foundational components, interfaces, and local cache functionality.

**Duration**: 2-3 weeks

**Status**: Not Started

## Overview

Phase 1 establishes the architectural foundation for GossipCache. We'll create core abstractions, implement a local cache storage engine, and set up the project infrastructure. By the end of this phase, you'll have a working single-node cache with excellent test coverage.

## Objectives

- [ ] Project structure and build system
- [ ] Core interfaces following SOLID principles
- [ ] Configuration management system
- [ ] Logging and observability foundation
- [ ] Local storage engine with concurrency control
- [ ] Basic cache operations (Get, Set, Delete, GetMulti, SetMulti)
- [ ] TTL and expiration handling
- [ ] Eviction policies (LRU)
- [ ] Comprehensive unit tests (>80% coverage)
- [ ] Performance benchmarks

## Package Structure

```
gossipcache/
├── cmd/
│   └── gossipcache/
│       └── main.go                 # Entry point
├── internal/
│   ├── cache/
│   │   ├── cache.go               # Cache interface and manager
│   │   ├── entry.go               # CacheEntry types
│   │   └── stats.go               # Statistics tracking
│   ├── storage/
│   │   ├── storage.go             # Storage interface
│   │   ├── memory/
│   │   │   ├── memory.go          # In-memory implementation
│   │   │   ├── sharded.go         # Sharded map for concurrency
│   │   │   └── eviction.go        # LRU eviction policy
│   │   └── mock/
│   │       └── mock_storage.go    # Mock for testing
│   ├── config/
│   │   ├── config.go              # Configuration structs
│   │   ├── loader.go              # Load from file/env
│   │   └── validator.go           # Config validation
│   ├── observability/
│   │   ├── logger.go              # Structured logging
│   │   └── metrics.go             # Metrics foundation
│   └── util/
│       ├── time.go                # Time utilities
│       ├── hash.go                # Hashing utilities
│       └── singleflight.go        # Singleflight pattern
├── pkg/
│   └── gossipcache/
│       ├── client.go              # Public client API
│       ├── errors.go              # Public errors
│       └── types.go               # Public types
├── test/
│   ├── integration/               # Integration tests (Phase 2+)
│   └── benchmark/
│       └── cache_bench_test.go    # Performance benchmarks
├── go.mod
├── go.sum
├── Makefile                       # Build automation
└── README.md
```

## Implementation Steps

### Step 1: Project Setup (Day 1-2)

#### 1.1 Initialize Project

```bash
# Go module is already initialized as github.com/sanketn26/gossipcache.
# For a fresh scaffold, run this only if go.mod does not exist:
# go mod init github.com/sanketn26/gossipcache

# Create directory structure
mkdir -p cmd/gossipcache
mkdir -p internal/{cache,storage/memory,config,observability,util}
mkdir -p pkg/gossipcache
mkdir -p test/{integration,benchmark}
```

#### 1.2 Create Makefile

```makefile
# Makefile
.PHONY: all build test coverage lint clean

all: lint test build

build:
	go build -o bin/gossipcache ./cmd/gossipcache

test:
	go test -v -race ./...

coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run

benchmark:
	go test -bench=. -benchmem ./test/benchmark/

clean:
	rm -rf bin/ coverage.out coverage.html
```

#### 1.3 Setup golangci-lint

```yaml
# .golangci.yml
linters:
  enable:
    - gofmt
    - golint
    - govet
    - errcheck
    - staticcheck
    - unused
    - gosimple
    - structcheck
    - varcheck
    - deadcode
    - typecheck
    - gocyclo
    - misspell

linters-settings:
  gocyclo:
    min-complexity: 15
```

### Step 2: Core Interfaces (Day 2-3)

**SOLID Principle**: Interface Segregation - Define small, focused interfaces

#### 2.1 Storage Interface

```go
// internal/storage/storage.go
package storage

import (
    "context"
    "time"
)

// Storage defines the interface for cache storage engines.
// SRP: Single responsibility - data storage and retrieval
type Storage interface {
    // Get retrieves a value by key
    Get(ctx context.Context, key string) (*Entry, error)

    // Set stores a value with TTL
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

    // Delete removes a key
    Delete(ctx context.Context, key string) error

    // GetMulti retrieves multiple keys (batch operation)
    GetMulti(ctx context.Context, keys []string) (map[string]*Entry, error)

    // SetMulti stores multiple entries (batch operation)
    SetMulti(ctx context.Context, entries map[string]*Entry) error

    // Keys returns all keys (for anti-entropy)
    Keys(ctx context.Context) ([]string, error)

    // Stats returns storage statistics
    Stats(ctx context.Context) (*Stats, error)

    // Close cleans up resources
    Close() error
}

// Entry represents a cache entry
type Entry struct {
    Key       string
    Value     []byte
    ExpiresAt time.Time
    CreatedAt time.Time
    UpdatedAt time.Time
}

// Stats represents storage statistics
type Stats struct {
    Keys      int64
    Size      int64 // bytes
    Hits      int64
    Misses    int64
    Evictions int64
}

// IsExpired checks if entry has expired
func (e *Entry) IsExpired() bool {
    if e.ExpiresAt.IsZero() {
        return false
    }
    return time.Now().After(e.ExpiresAt)
}
```

#### 2.2 Cache Interface

```go
// pkg/gossipcache/client.go
package gossipcache

import (
    "context"
    "time"
)

// Cache is the main interface for cache operations
// ISP: Interface segregation - client-facing interface
type Cache interface {
    // Get retrieves a value by key
    Get(ctx context.Context, key string) ([]byte, error)

    // Set stores a value with TTL
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

    // Delete removes a key
    Delete(ctx context.Context, key string) error

    // GetMulti retrieves multiple keys
    GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)

    // SetMulti stores multiple entries
    SetMulti(ctx context.Context, entries map[string][]byte, ttl time.Duration) error

    // Flush removes all entries
    Flush(ctx context.Context) error

    // Stats returns cache statistics
    Stats(ctx context.Context) (*CacheStats, error)

    // Close gracefully shuts down the cache
    Close() error
}

// CacheStats represents cache statistics
type CacheStats struct {
    Hits      int64
    Misses    int64
    Evictions int64
    Size      int64
    Keys      int64
}
```

#### 2.3 Error Types

```go
// pkg/gossipcache/errors.go
package gossipcache

import "errors"

var (
    // ErrKeyNotFound indicates the key was not found
    ErrKeyNotFound = errors.New("key not found")

    // ErrKeyTooLarge indicates the key exceeds maximum size
    ErrKeyTooLarge = errors.New("key too large")

    // ErrValueTooLarge indicates the value exceeds maximum size
    ErrValueTooLarge = errors.New("value too large")

    // ErrCacheFull indicates the cache is at capacity
    ErrCacheFull = errors.New("cache full")

    // ErrClosed indicates the cache is closed
    ErrClosed = errors.New("cache closed")
)
```

### Step 3: Configuration Management (Day 3-4)

**SOLID Principle**: Single Responsibility - Config has one job: manage configuration

#### 3.1 Configuration Structure

```go
// internal/config/config.go
package config

import "time"

// Config represents the complete configuration
type Config struct {
    Mode      OperatingMode `yaml:"mode" env:"MODE"`
    NodeID    string        `yaml:"node_id" env:"NODE_ID"`
    Address   string        `yaml:"address" env:"ADDRESS"`

    Cache     CacheConfig     `yaml:"cache"`
    Gossip    GossipConfig    `yaml:"gossip"`
    Network   NetworkConfig   `yaml:"network"`
    Logging   LoggingConfig   `yaml:"logging"`
    Metrics   MetricsConfig   `yaml:"metrics"`
}

type OperatingMode string

const (
    ModeBacked      OperatingMode = "backed"
    ModeIndependent OperatingMode = "independent"
)

type CacheConfig struct {
    MaxSize        int64           `yaml:"max_size" env:"CACHE_MAX_SIZE"`
    DefaultTTL     time.Duration   `yaml:"default_ttl" env:"CACHE_DEFAULT_TTL"`
    EvictionPolicy string          `yaml:"eviction_policy" env:"CACHE_EVICTION_POLICY"`
    MaxKeySize     int             `yaml:"max_key_size"`
    MaxValueSize   int             `yaml:"max_value_size"`
}

type GossipConfig struct {
    Interval            time.Duration `yaml:"interval"`
    Fanout              int           `yaml:"fanout"`
    AntiEntropyInterval time.Duration `yaml:"anti_entropy_interval"`
}

type NetworkConfig struct {
    TCPPort int `yaml:"tcp_port" env:"TCP_PORT"`
    UDPPort int `yaml:"udp_port" env:"UDP_PORT"`
}

type LoggingConfig struct {
    Level  string `yaml:"level" env:"LOG_LEVEL"`
    Format string `yaml:"format" env:"LOG_FORMAT"` // json or text
}

type MetricsConfig struct {
    Enabled bool `yaml:"enabled" env:"METRICS_ENABLED"`
    Port    int  `yaml:"port" env:"METRICS_PORT"`
}

// Default returns a configuration with sensible defaults
func Default() *Config {
    return &Config{
        Mode:    ModeBacked,
        NodeID:  "", // Will be set to hostname
        Address: "0.0.0.0:7946",
        Cache: CacheConfig{
            MaxSize:        1 << 30, // 1GB
            DefaultTTL:     5 * time.Minute,
            EvictionPolicy: "lru",
            MaxKeySize:     1024,      // 1KB
            MaxValueSize:   10 << 20,  // 10MB
        },
        Gossip: GossipConfig{
            Interval:            1 * time.Second,
            Fanout:              3,
            AntiEntropyInterval: 5 * time.Minute,
        },
        Network: NetworkConfig{
            TCPPort: 7946,
            UDPPort: 7946,
        },
        Logging: LoggingConfig{
            Level:  "info",
            Format: "json",
        },
        Metrics: MetricsConfig{
            Enabled: true,
            Port:    9090,
        },
    }
}
```

#### 3.2 Configuration Loader

```go
// internal/config/loader.go
package config

import (
    "fmt"
    "os"
    "gopkg.in/yaml.v3"
)

// Load loads configuration from file and environment
// DRY: Single function handles all config loading
func Load(path string) (*Config, error) {
    cfg := Default()

    // Load from file if provided
    if path != "" {
        if err := loadFromFile(cfg, path); err != nil {
            return nil, fmt.Errorf("load config file: %w", err)
        }
    }

    // Override with environment variables
    if err := loadFromEnv(cfg); err != nil {
        return nil, fmt.Errorf("load config from env: %w", err)
    }

    // Validate
    if err := Validate(cfg); err != nil {
        return nil, fmt.Errorf("validate config: %w", err)
    }

    return cfg, nil
}

func loadFromFile(cfg *Config, path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()

    decoder := yaml.NewDecoder(f)
    return decoder.Decode(cfg)
}

func loadFromEnv(cfg *Config) error {
    // Use reflection or manual parsing
    // For simplicity, manual parsing shown

    if mode := os.Getenv("MODE"); mode != "" {
        cfg.Mode = OperatingMode(mode)
    }

    if nodeID := os.Getenv("NODE_ID"); nodeID != "" {
        cfg.NodeID = nodeID
    }

    // ... more environment variable parsing

    return nil
}
```

#### 3.3 Configuration Validator

```go
// internal/config/validator.go
package config

import (
    "errors"
    "fmt"
    "os"
)

// Validate validates the configuration
func Validate(cfg *Config) error {
    // Validate mode
    if cfg.Mode != ModeBacked && cfg.Mode != ModeIndependent {
        return fmt.Errorf("invalid mode: %s (must be 'backed' or 'independent')", cfg.Mode)
    }

    // Set NodeID if empty
    if cfg.NodeID == "" {
        hostname, err := os.Hostname()
        if err != nil {
            return fmt.Errorf("get hostname for node_id: %w", err)
        }
        cfg.NodeID = hostname
    }

    // Validate cache settings
    if cfg.Cache.MaxSize <= 0 {
        return errors.New("cache.max_size must be positive")
    }

    if cfg.Cache.DefaultTTL < 0 {
        return errors.New("cache.default_ttl cannot be negative")
    }

    // Validate gossip settings
    if cfg.Gossip.Fanout <= 0 {
        return errors.New("gossip.fanout must be positive")
    }

    // Validate network ports
    if cfg.Network.TCPPort <= 0 || cfg.Network.TCPPort > 65535 {
        return fmt.Errorf("invalid tcp_port: %d", cfg.Network.TCPPort)
    }

    return nil
}
```

### Step 4: Logging Foundation (Day 4)

**SOLID Principle**: Dependency Inversion - Depend on logger interface, not concrete implementation

```go
// internal/observability/logger.go
package observability

import (
    "log/slog"
    "os"
)

// Logger wraps slog for structured logging
type Logger struct {
    *slog.Logger
}

// NewLogger creates a new logger with the given configuration
func NewLogger(level, format string) *Logger {
    var handler slog.Handler

    var logLevel slog.Level
    switch level {
    case "debug":
        logLevel = slog.LevelDebug
    case "info":
        logLevel = slog.LevelInfo
    case "warn":
        logLevel = slog.LevelWarn
    case "error":
        logLevel = slog.LevelError
    default:
        logLevel = slog.LevelInfo
    }

    opts := &slog.HandlerOptions{
        Level: logLevel,
    }

    if format == "json" {
        handler = slog.NewJSONHandler(os.Stdout, opts)
    } else {
        handler = slog.NewTextHandler(os.Stdout, opts)
    }

    return &Logger{
        Logger: slog.New(handler),
    }
}

// WithComponent returns a logger with a component field
func (l *Logger) WithComponent(component string) *Logger {
    return &Logger{
        Logger: l.With("component", component),
    }
}
```

### Step 5: Local Storage Implementation (Day 5-8)

**SOLID Principle**: Open/Closed - Open for extension (different eviction policies), closed for modification

#### 5.1 Sharded Map for Concurrency

```go
// internal/storage/memory/sharded.go
package memory

import (
    "context"
    "hash/fnv"
    "sync"

    "github.com/sanketn26/gossipcache/internal/storage"
)

const defaultShards = 256

// shardedMap provides concurrent access to cache entries
// DRY: Encapsulates sharding logic
type shardedMap struct {
    shards []*shard
    count  int
}

type shard struct {
    mu      sync.RWMutex
    entries map[string]*storage.Entry
}

func newShardedMap(numShards int) *shardedMap {
    if numShards <= 0 {
        numShards = defaultShards
    }

    sm := &shardedMap{
        shards: make([]*shard, numShards),
        count:  numShards,
    }

    for i := 0; i < numShards; i++ {
        sm.shards[i] = &shard{
            entries: make(map[string]*storage.Entry),
        }
    }

    return sm
}

func (sm *shardedMap) getShard(key string) *shard {
    h := fnv.New32a()
    h.Write([]byte(key))
    return sm.shards[h.Sum32()%uint32(sm.count)]
}

func (sm *shardedMap) get(key string) (*storage.Entry, bool) {
    s := sm.getShard(key)
    s.mu.RLock()
    defer s.mu.RUnlock()

    entry, ok := s.entries[key]
    return entry, ok
}

func (sm *shardedMap) set(key string, entry *storage.Entry) {
    s := sm.getShard(key)
    s.mu.Lock()
    defer s.mu.Unlock()

    s.entries[key] = entry
}

func (sm *shardedMap) delete(key string) {
    s := sm.getShard(key)
    s.mu.Lock()
    defer s.mu.Unlock()

    delete(s.entries, key)
}

func (sm *shardedMap) keys() []string {
    keys := make([]string, 0)

    for _, s := range sm.shards {
        s.mu.RLock()
        for k := range s.entries {
            keys = append(keys, k)
        }
        s.mu.RUnlock()
    }

    return keys
}

func (sm *shardedMap) len() int {
    total := 0
    for _, s := range sm.shards {
        s.mu.RLock()
        total += len(s.entries)
        s.mu.RUnlock()
    }
    return total
}
```

#### 5.2 Memory Storage Implementation

```go
// internal/storage/memory/memory.go
package memory

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
    "time"

    "github.com/sanketn26/gossipcache/internal/storage"
)

// MemoryStorage implements in-memory cache storage
// SRP: Responsible only for storing and retrieving data
type MemoryStorage struct {
    data      *shardedMap
    stats     *storageStats
    maxSize   int64
    eviction  EvictionPolicy
    closed    atomic.Bool
    closeCh   chan struct{}
    wg        sync.WaitGroup
}

type storageStats struct {
    hits      atomic.Int64
    misses    atomic.Int64
    evictions atomic.Int64
}

// New creates a new MemoryStorage
func New(maxSize int64, evictionPolicy string) (*MemoryStorage, error) {
    eviction, err := newEvictionPolicy(evictionPolicy)
    if err != nil {
        return nil, err
    }

    ms := &MemoryStorage{
        data:     newShardedMap(256),
        stats:    &storageStats{},
        maxSize:  maxSize,
        eviction: eviction,
        closeCh:  make(chan struct{}),
    }

    // Start expiration goroutine
    ms.wg.Add(1)
    go ms.expirationLoop()

    return ms, nil
}

func (ms *MemoryStorage) Get(ctx context.Context, key string) (*storage.Entry, error) {
    if ms.closed.Load() {
        return nil, storage.ErrClosed
    }

    entry, ok := ms.data.get(key)
    if !ok {
        ms.stats.misses.Add(1)
        return nil, storage.ErrKeyNotFound
    }

    // Check expiration
    if entry.IsExpired() {
        ms.data.delete(key)
        ms.stats.misses.Add(1)
        return nil, storage.ErrKeyNotFound
    }

    ms.stats.hits.Add(1)
    ms.eviction.OnAccess(key)

    return entry, nil
}

func (ms *MemoryStorage) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    if ms.closed.Load() {
        return storage.ErrClosed
    }

    now := time.Now()
    entry := &storage.Entry{
        Key:       key,
        Value:     value,
        CreatedAt: now,
        UpdatedAt: now,
    }

    if ttl > 0 {
        entry.ExpiresAt = now.Add(ttl)
    }

    // Check if we need to evict
    if ms.shouldEvict() {
        if err := ms.evict(); err != nil {
            return err
        }
    }

    ms.data.set(key, entry)
    ms.eviction.OnAdd(key)

    return nil
}

func (ms *MemoryStorage) Delete(ctx context.Context, key string) error {
    if ms.closed.Load() {
        return storage.ErrClosed
    }

    ms.data.delete(key)
    ms.eviction.OnRemove(key)

    return nil
}

func (ms *MemoryStorage) GetMulti(ctx context.Context, keys []string) (map[string]*storage.Entry, error) {
    result := make(map[string]*storage.Entry)

    for _, key := range keys {
        entry, err := ms.Get(ctx, key)
        if err == nil {
            result[key] = entry
        }
    }

    return result, nil
}

func (ms *MemoryStorage) SetMulti(ctx context.Context, entries map[string]*storage.Entry) error {
    for key, entry := range entries {
        ttl := time.Until(entry.ExpiresAt)
        if err := ms.Set(ctx, key, entry.Value, ttl); err != nil {
            return err
        }
    }
    return nil
}

func (ms *MemoryStorage) Keys(ctx context.Context) ([]string, error) {
    if ms.closed.Load() {
        return nil, storage.ErrClosed
    }

    return ms.data.keys(), nil
}

func (ms *MemoryStorage) Stats(ctx context.Context) (*storage.Stats, error) {
    return &storage.Stats{
        Keys:      int64(ms.data.len()),
        Size:      ms.currentSize(),
        Hits:      ms.stats.hits.Load(),
        Misses:    ms.stats.misses.Load(),
        Evictions: ms.stats.evictions.Load(),
    }, nil
}

func (ms *MemoryStorage) Close() error {
    if ms.closed.Swap(true) {
        return nil // Already closed
    }

    close(ms.closeCh)
    ms.wg.Wait()

    return nil
}

func (ms *MemoryStorage) shouldEvict() bool {
    return ms.currentSize() > ms.maxSize
}

func (ms *MemoryStorage) currentSize() int64 {
    // Approximate size calculation
    keys := ms.data.keys()
    var size int64

    for _, key := range keys {
        entry, ok := ms.data.get(key)
        if ok {
            size += int64(len(key) + len(entry.Value))
        }
    }

    return size
}

func (ms *MemoryStorage) evict() error {
    victim := ms.eviction.SelectVictim()
    if victim == "" {
        return fmt.Errorf("no victim to evict")
    }

    ms.data.delete(victim)
    ms.eviction.OnRemove(victim)
    ms.stats.evictions.Add(1)

    return nil
}

func (ms *MemoryStorage) expirationLoop() {
    defer ms.wg.Done()

    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ms.closeCh:
            return
        case <-ticker.C:
            ms.removeExpired()
        }
    }
}

func (ms *MemoryStorage) removeExpired() {
    keys := ms.data.keys()

    for _, key := range keys {
        entry, ok := ms.data.get(key)
        if ok && entry.IsExpired() {
            ms.data.delete(key)
            ms.eviction.OnRemove(key)
        }
    }
}
```

#### 5.3 LRU Eviction Policy

```go
// internal/storage/memory/eviction.go
package memory

import (
    "container/list"
    "errors"
    "sync"
)

// EvictionPolicy defines the interface for eviction policies
// OCP: Open for extension (add new policies), closed for modification
type EvictionPolicy interface {
    OnAdd(key string)
    OnAccess(key string)
    OnRemove(key string)
    SelectVictim() string
}

func newEvictionPolicy(policy string) (EvictionPolicy, error) {
    switch policy {
    case "lru":
        return newLRUPolicy(), nil
    default:
        return nil, errors.New("unsupported eviction policy")
    }
}

// LRU eviction policy
type lruPolicy struct {
    mu       sync.Mutex
    list     *list.List
    elements map[string]*list.Element
}

func newLRUPolicy() *lruPolicy {
    return &lruPolicy{
        list:     list.New(),
        elements: make(map[string]*list.Element),
    }
}

func (lru *lruPolicy) OnAdd(key string) {
    lru.mu.Lock()
    defer lru.mu.Unlock()

    if elem, ok := lru.elements[key]; ok {
        lru.list.MoveToFront(elem)
        return
    }

    elem := lru.list.PushFront(key)
    lru.elements[key] = elem
}

func (lru *lruPolicy) OnAccess(key string) {
    lru.mu.Lock()
    defer lru.mu.Unlock()

    if elem, ok := lru.elements[key]; ok {
        lru.list.MoveToFront(elem)
    }
}

func (lru *lruPolicy) OnRemove(key string) {
    lru.mu.Lock()
    defer lru.mu.Unlock()

    if elem, ok := lru.elements[key]; ok {
        lru.list.Remove(elem)
        delete(lru.elements, key)
    }
}

func (lru *lruPolicy) SelectVictim() string {
    lru.mu.Lock()
    defer lru.mu.Unlock()

    elem := lru.list.Back()
    if elem == nil {
        return ""
    }

    return elem.Value.(string)
}
```

### Step 6: Cache Manager (Day 9-10)

```go
// internal/cache/cache.go
package cache

import (
    "context"
    "time"

    "github.com/sanketn26/gossipcache/internal/storage"
    "github.com/sanketn26/gossipcache/pkg/gossipcache"
)

// Manager implements the Cache interface
// SRP: Coordinates between storage and higher-level cache operations
type Manager struct {
    storage storage.Storage
    config  *CacheConfig
}

type CacheConfig struct {
    DefaultTTL time.Duration
}

func NewManager(storage storage.Storage, config *CacheConfig) *Manager {
    return &Manager{
        storage: storage,
        config:  config,
    }
}

func (m *Manager) Get(ctx context.Context, key string) ([]byte, error) {
    entry, err := m.storage.Get(ctx, key)
    if err != nil {
        return nil, err
    }

    return entry.Value, nil
}

func (m *Manager) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    if ttl == 0 {
        ttl = m.config.DefaultTTL
    }

    return m.storage.Set(ctx, key, value, ttl)
}

func (m *Manager) Delete(ctx context.Context, key string) error {
    return m.storage.Delete(ctx, key)
}

func (m *Manager) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
    entries, err := m.storage.GetMulti(ctx, keys)
    if err != nil {
        return nil, err
    }

    result := make(map[string][]byte, len(entries))
    for k, entry := range entries {
        result[k] = entry.Value
    }

    return result, nil
}

func (m *Manager) SetMulti(ctx context.Context, entries map[string][]byte, ttl time.Duration) error {
    if ttl == 0 {
        ttl = m.config.DefaultTTL
    }

    storageEntries := make(map[string]*storage.Entry)
    now := time.Now()

    for k, v := range entries {
        storageEntries[k] = &storage.Entry{
            Key:       k,
            Value:     v,
            CreatedAt: now,
            UpdatedAt: now,
            ExpiresAt: now.Add(ttl),
        }
    }

    return m.storage.SetMulti(ctx, storageEntries)
}

func (m *Manager) Flush(ctx context.Context) error {
    keys, err := m.storage.Keys(ctx)
    if err != nil {
        return err
    }

    for _, key := range keys {
        if err := m.storage.Delete(ctx, key); err != nil {
            return err
        }
    }

    return nil
}

func (m *Manager) Stats(ctx context.Context) (*gossipcache.CacheStats, error) {
    stats, err := m.storage.Stats(ctx)
    if err != nil {
        return nil, err
    }

    return &gossipcache.CacheStats{
        Hits:      stats.Hits,
        Misses:    stats.Misses,
        Evictions: stats.Evictions,
        Size:      stats.Size,
        Keys:      stats.Keys,
    }, nil
}

func (m *Manager) Close() error {
    return m.storage.Close()
}
```

### Step 7: Unit Tests (Day 11-12)

```go
// internal/storage/memory/memory_test.go
package memory

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMemoryStorage_GetSet(t *testing.T) {
    storage, err := New(1<<20, "lru") // 1MB
    require.NoError(t, err)
    defer storage.Close()

    ctx := context.Background()

    // Test Set
    err = storage.Set(ctx, "key1", []byte("value1"), 5*time.Minute)
    require.NoError(t, err)

    // Test Get
    entry, err := storage.Get(ctx, "key1")
    require.NoError(t, err)
    assert.Equal(t, "key1", entry.Key)
    assert.Equal(t, []byte("value1"), entry.Value)
}

func TestMemoryStorage_Expiration(t *testing.T) {
    storage, err := New(1<<20, "lru")
    require.NoError(t, err)
    defer storage.Close()

    ctx := context.Background()

    // Set with short TTL
    err = storage.Set(ctx, "key1", []byte("value1"), 100*time.Millisecond)
    require.NoError(t, err)

    // Should exist immediately
    _, err = storage.Get(ctx, "key1")
    require.NoError(t, err)

    // Wait for expiration
    time.Sleep(150 * time.Millisecond)

    // Should be expired
    _, err = storage.Get(ctx, "key1")
    assert.Error(t, err)
}

func TestMemoryStorage_Eviction(t *testing.T) {
    // Small cache for testing eviction
    storage, err := New(100, "lru") // 100 bytes
    require.NoError(t, err)
    defer storage.Close()

    ctx := context.Background()

    // Fill cache beyond capacity
    for i := 0; i < 10; i++ {
        key := fmt.Sprintf("key%d", i)
        value := []byte(strings.Repeat("x", 20)) // 20 bytes
        err = storage.Set(ctx, key, value, 5*time.Minute)
        require.NoError(t, err)
    }

    // Check stats
    stats, err := storage.Stats(ctx)
    require.NoError(t, err)
    assert.Greater(t, stats.Evictions, int64(0))
}

// Benchmark tests
func BenchmarkMemoryStorage_Get(b *testing.B) {
    storage, _ := New(1<<30, "lru") // 1GB
    defer storage.Close()

    ctx := context.Background()
    storage.Set(ctx, "key1", []byte("value1"), 5*time.Minute)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        storage.Get(ctx, "key1")
    }
}

func BenchmarkMemoryStorage_Set(b *testing.B) {
    storage, _ := New(1<<30, "lru")
    defer storage.Close()

    ctx := context.Background()
    value := []byte("value1")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        key := fmt.Sprintf("key%d", i)
        storage.Set(ctx, key, value, 5*time.Minute)
    }
}
```

### Step 8: Integration Example (Day 13-14)

```go
// cmd/gossipcache/main.go
package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/sanketn26/gossipcache/internal/cache"
    "github.com/sanketn26/gossipcache/internal/config"
    "github.com/sanketn26/gossipcache/internal/observability"
    "github.com/sanketn26/gossipcache/internal/storage/memory"
)

func main() {
    configPath := flag.String("config", "config.yaml", "Path to configuration file")
    flag.Parse()

    // Load configuration
    cfg, err := config.Load(*configPath)
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Setup logger
    logger := observability.NewLogger(cfg.Logging.Level, cfg.Logging.Format)
    logger.Info("starting gossipcache",
        "node_id", cfg.NodeID,
        "mode", cfg.Mode,
    )

    // Create storage
    storage, err := memory.New(cfg.Cache.MaxSize, cfg.Cache.EvictionPolicy)
    if err != nil {
        logger.Error("failed to create storage", "error", err)
        os.Exit(1)
    }

    // Create cache manager
    cacheManager := cache.NewManager(storage, &cache.CacheConfig{
        DefaultTTL: cfg.Cache.DefaultTTL,
    })

    // Example: Set and Get
    ctx := context.Background()

    if err := cacheManager.Set(ctx, "hello", []byte("world"), 0); err != nil {
        logger.Error("failed to set key", "error", err)
    }

    value, err := cacheManager.Get(ctx, "hello")
    if err != nil {
        logger.Error("failed to get key", "error", err)
    } else {
        logger.Info("retrieved value", "key", "hello", "value", string(value))
    }

    // Wait for interrupt
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    <-sigCh

    logger.Info("shutting down")

    if err := cacheManager.Close(); err != nil {
        logger.Error("error closing cache", "error", err)
    }
}
```

## Testing Strategy

### Unit Tests
- Test each component in isolation
- Mock dependencies using interfaces
- Aim for >80% code coverage

### Benchmarks
- Get/Set operations should be < 1ms
- Concurrent access benchmarks
- Memory usage profiling

## Deliverables

- [ ] Complete package structure
- [ ] Core interfaces defined
- [ ] Configuration system working
- [ ] Local storage with LRU eviction
- [ ] Cache manager implementation
- [ ] Unit tests with >80% coverage
- [ ] Benchmarks showing < 1ms operations
- [ ] Working main.go example
- [ ] Documentation updated

## Success Criteria

1. **Functional**: Single-node cache works with Get/Set/Delete
2. **Performance**: < 1ms for Get/Set operations
3. **Quality**: >80% test coverage, all tests passing
4. **Clean Code**: SOLID principles applied, interfaces well-defined
5. **Documentation**: README updated, code documented

## Next Phase

Once Phase 1 is complete, move to [Phase 2: Backed Mode](PHASE_2_BACKED_MODE.md) to add Redis integration and gossip protocol.

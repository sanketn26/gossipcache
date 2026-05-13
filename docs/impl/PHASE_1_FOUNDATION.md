# Phase 1: Core Foundation

**Goal**: Build the foundational components, interfaces, and local cache functionality.

**Duration**: 2-3 weeks

**Status**: Complete

## Overview

Phase 1 establishes the architectural foundation for GossipCache. We'll create core abstractions, implement a local cache storage engine, and set up the project infrastructure. By the end of this phase, you'll have a working single-node cache with excellent test coverage.

## Objectives

- [x] Project structure and build system
- [x] Core interfaces following SOLID principles
- [x] Configuration management system
- [x] Logging and Prometheus observability foundation
- [x] Local storage engine with concurrency control
- [x] Basic cache operations (Get, Set, Delete, GetMulti, SetMulti)
- [x] TTL and expiration handling
- [x] Eviction policies (LRU)
- [x] Comprehensive unit tests (>80% coverage)
- [x] Performance benchmarks

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

### Phase 1 TDD Rhythm

For each subsection below, write the listed test before the production file. Run the narrow package command after every passing test, then run `make test-short` at the end of each day. A slice is complete only when the test fails for the expected reason first, then passes with the smallest implementation.

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

### Step 4: Logging and Prometheus Observability Foundation (Day 4)

**SOLID Principle**: Dependency Inversion - Depend on logger interface, not concrete implementation

Start with structured logging and a small Prometheus metrics wrapper. Phase 1 should not expose every final production metric, but it should establish the registry, naming conventions, and cache-level counters/gauges that later phases can extend for gossip, backing stores, and network transport.

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

#### 4.1 Prometheus Metrics Foundation

Add Prometheus client support in Phase 1 so cache behavior is observable from the first working single-node implementation.

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

```go
// internal/observability/metrics.go
package observability

import (
    "net/http"
    "strconv"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "gossipcache"

// Metrics owns cache-level Prometheus collectors.
// Later phases add gossip, network, and backing-store collectors here.
type Metrics struct {
    registry *prometheus.Registry

    cacheOperations *prometheus.CounterVec
    cacheSizeBytes  prometheus.Gauge
    cacheKeys       prometheus.Gauge
}

func NewMetrics(registry *prometheus.Registry) *Metrics {
    if registry == nil {
        registry = prometheus.NewRegistry()
    }

    m := &Metrics{
        registry: registry,
        cacheOperations: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "cache_operations_total",
                Help:      "Total cache operations by operation and result.",
            },
            []string{"operation", "result"},
        ),
        cacheSizeBytes: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Namespace: namespace,
                Name:      "cache_size_bytes",
                Help:      "Current cache size in bytes.",
            },
        ),
        cacheKeys: prometheus.NewGauge(
            prometheus.GaugeOpts{
                Namespace: namespace,
                Name:      "cache_keys",
                Help:      "Current number of cache keys.",
            },
        ),
    }

    registry.MustRegister(m.cacheOperations, m.cacheSizeBytes, m.cacheKeys)
    return m
}

func (m *Metrics) RecordCacheOperation(operation string, err error) {
    result := "success"
    if err != nil {
        result = "error"
    }
    m.cacheOperations.WithLabelValues(operation, result).Inc()
}

func (m *Metrics) SetCacheStats(sizeBytes, keys int64) {
    m.cacheSizeBytes.Set(float64(sizeBytes))
    m.cacheKeys.Set(float64(keys))
}

func (m *Metrics) Handler() http.Handler {
    return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Address(port int) string {
    return ":" + strconv.Itoa(port)
}
```

Configuration should control whether the metrics endpoint starts:

```go
// cmd/gossipcache/main.go
if cfg.Metrics.Enabled {
    metrics := observability.NewMetrics(nil)
    go func() {
        mux := http.NewServeMux()
        mux.Handle("/metrics", metrics.Handler())
        if err := http.ListenAndServe(metrics.Address(cfg.Metrics.Port), mux); err != nil {
            logger.Error("metrics server stopped", "error", err)
        }
    }()
}
```

TDD slices:
- `internal/observability/metrics_test.go`: registers collectors on an injected registry without duplicate global registrations.
- `internal/observability/metrics_test.go`: records cache operation counters with `operation` and `result` labels.
- `internal/observability/metrics_test.go`: updates cache size and key gauges.
- `internal/observability/metrics_test.go`: `/metrics` handler exposes the `gossipcache_` metric family names.

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

### Step 7: Unit Tests and Regression Pass (Day 11-12)

Completed in the repository with focused, dependency-free Go tests:

- `internal/storage/memory/memory_test.go`: set/get copies values, missing keys return `storage.ErrKeyNotFound`, expired keys are removed on access, delete removes keys, stats update, close is idempotent, batch operations work, and concurrent access is race-safe.
- `internal/storage/memory/eviction_test.go`: LRU selects the least recently used key and moves existing keys to the front on re-add.
- `internal/storage/memory/sharded_test.go`: invalid shard counts fall back to `defaultShards`, and concurrent map access remains safe.
- `internal/cache/cache_test.go`: manager-level `Get`, `Set`, `Delete`, `GetMulti`, `SetMulti`, `Flush`, `Stats`, explicit TTLs, and `Close` are covered through the storage interface.
- Existing tests cover public API contracts, public errors, service lifecycle, config defaults/loading/validation, storage entry expiration, logger/metrics setup, and metrics service lifecycle.

Regression fixes made while completing this step:

- `MemoryStorage.Set` now copies the caller's value slice before storing it.
- `MemoryStorage.Get` now returns a copied entry so callers cannot mutate cached data through the returned slice.
- `MemoryStorage.Set` evicts after insertion until the cache is back under `maxSize`, so a newly added entry cannot leave the cache permanently above capacity.

### Step 8: Integration Example (Day 13-14)

Completed in `cmd/gossipcache/main.go`.

The command now:

- Loads config from an optional `-config` flag, defaulting to built-in config when omitted.
- Creates the structured logger from config.
- Starts the metrics service through `gossipcache.ServiceRegistry`.
- Creates the in-memory storage engine and cache manager.
- Exercises a simple `Set`/`Get` example at startup.
- Waits for `SIGINT` or `SIGTERM`.
- Gracefully closes the cache and shuts down registered services.

Run it with:

```bash
go run ./cmd/gossipcache
```

Or with a config file:

```bash
go run ./cmd/gossipcache -config config.yaml
```

## Testing Strategy

### TDD Test Plan

| Slice | Write This Test First | Expected Behavior | Checkpoint |
| --- | --- | --- | --- |
| Public API | `pkg/gossipcache/client_test.go` | A concrete type can satisfy the exported `Cache` interface without importing internals | `go test ./pkg/gossipcache` |
| Public errors | `pkg/gossipcache/errors_test.go` | Sentinel errors have stable messages and remain comparable with `errors.Is` | `go test ./pkg/gossipcache` |
| Defaults | `internal/config/config_test.go` | `Default()` returns a valid backed-mode single-node config | `go test ./internal/config` |
| Config loading | `internal/config/loader_test.go` | YAML values load, supported env vars override file values, file errors are wrapped | `go test ./internal/config` |
| Config validation | `internal/config/validator_test.go` | Invalid mode, cache size, TTL, gossip fanout, and TCP port are rejected | `go test ./internal/config` |
| Entry expiration | `internal/storage/storage_test.go` | Zero expiration never expires, past expiration does, future expiration does not | `go test ./internal/storage` |
| Memory storage | `internal/storage/memory/memory_test.go` | Set/Get returns copies, missing/expired keys return not found, delete removes keys | `go test -race ./internal/storage/...` |
| Eviction | `internal/storage/memory/eviction_test.go` | LRU evicts the least recently used entry when size is exceeded | `go test ./internal/storage/memory` |
| Cache manager | `internal/cache/local_cache_test.go` | Get/Set/Delete/GetMulti/SetMulti map storage behavior to public cache behavior | `go test ./internal/cache ./internal/storage/...` |
| Stats | `internal/cache/stats_test.go` | Hits, misses, evictions, size, and key counts update deterministically | `go test ./internal/cache` |
| Observability | `internal/observability/*_test.go` | Logger setup is deterministic, Prometheus collectors register on an injected registry, cache counters/gauges update, and `/metrics` exposes `gossipcache_` metrics | `go test ./internal/observability` |

### Unit Test Rules
- Test each component in isolation before wiring it into the next package.
- Prefer table-driven tests for validation, expiration, eviction, and error mapping.
- Use fakes for package boundaries until the real dependency has its own tests.
- Aim for >80% code coverage, but treat behavior coverage as the gate.

### Benchmarks
- `test/benchmark/cache_bench_test.go` covers cache manager `Get`, `Set`, and parallel mixed `Get`/`Set` workloads.
- Run benchmarks with `go test -bench=. -benchmem ./...`.
- Get/Set operations should remain under 1ms on normal development hardware.
- CPU, memory, block, and trace profiling targets are available through the Makefile.

## Deliverables

- [x] Complete package structure
- [x] Core interfaces defined
- [x] Configuration system working
- [x] Local storage with LRU eviction
- [x] Cache manager implementation
- [x] Unit tests with >80% coverage
- [x] Benchmarks showing < 1ms operations
- [x] Working main.go example
- [x] Documentation updated

## Success Criteria

1. **Functional**: Single-node cache works with Get/Set/Delete
2. **Performance**: < 1ms for Get/Set operations
3. **Quality**: >80% test coverage, all tests passing
4. **Clean Code**: SOLID principles applied, interfaces well-defined
5. **Documentation**: README updated, code documented

## Next Phase

Once Phase 1 is complete, move to [Phase 2: Backed Mode](PHASE_2_BACKED_MODE.md) to add Redis integration and gossip protocol.

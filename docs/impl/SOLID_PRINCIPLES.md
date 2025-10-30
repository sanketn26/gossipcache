# SOLID Principles in GossipCache

This document explains how SOLID principles are applied throughout the GossipCache implementation.

## Overview

SOLID principles guide the design decisions in GossipCache, ensuring the codebase is maintainable, testable, and extensible.

## Single Responsibility Principle (SRP)

> A class should have one, and only one, reason to change.

### Examples in GossipCache

#### 1. Storage Engine
```go
// internal/storage/storage.go
type Storage interface {
    Get(ctx context.Context, key string) (*Entry, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
}
```

**Responsibility**: Data storage and retrieval only
**Does NOT**: Handle networking, gossip, or backing store logic

#### 2. Gossip Engine
```go
// internal/gossip/engine.go
type Engine struct {
    // Coordinates gossip protocol
}
```

**Responsibility**: Gossip protocol coordination
**Does NOT**: Handle storage, conflict resolution, or application logic

#### 3. Backing Store Connector
```go
// internal/backingstore/redis/redis.go
type RedisStore struct {
    // Redis communication only
}
```

**Responsibility**: Redis communication
**Does NOT**: Handle caching logic, gossip, or general storage interface

### Benefits
- **Easy Testing**: Each component can be tested in isolation
- **Clear Boundaries**: Each package has a well-defined purpose
- **Maintainability**: Changes to one component don't ripple across system

---

## Open/Closed Principle (OCP)

> Software entities should be open for extension, but closed for modification.

### Examples in GossipCache

#### 1. Eviction Policies
```go
// internal/storage/memory/eviction.go
type EvictionPolicy interface {
    OnAdd(key string)
    OnAccess(key string)
    OnRemove(key string)
    SelectVictim() string
}

// Add new policy without modifying existing code
type LRUPolicy struct {}
type LFUPolicy struct {}  // Can add without changing LRU
type TTLPolicy struct {}  // Can add without changing others
```

**Extension Point**: New eviction policies
**Closed to Modification**: Existing policies unchanged

#### 2. Backing Stores
```go
// internal/backingstore/backingstore.go
type BackingStore interface {
    Get(ctx context.Context, key string) ([]byte, int64, error)
    Set(ctx context.Context, key string, value []byte) (int64, error)
}

// Implementations
type RedisStore struct {}
type PostgresStore struct {}
type MySQLStore struct {}
```

**Extension Point**: New backing stores
**Closed to Modification**: Cache manager doesn't change

#### 3. Conflict Resolution Strategies
```go
// internal/conflict/resolver.go
type Resolver interface {
    Resolve(local, remote *VersionedEntry) (*VersionedEntry, error)
}

type LWWResolver struct {}
type CustomResolver struct {}
type SiblingsResolver struct {}
```

**Extension Point**: New resolution strategies
**Closed to Modification**: Gossip engine doesn't change

### Benefits
- **Extensibility**: New features without touching existing code
- **Stability**: Existing functionality remains stable
- **Plugin Architecture**: Easy to add new components

---

## Liskov Substitution Principle (LSP)

> Objects should be replaceable with instances of their subtypes without altering correctness.

### Examples in GossipCache

#### 1. Storage Implementations
```go
// Any storage implementation should work identically
func TestCache(storage storage.Storage) {
    storage.Set(ctx, "key", []byte("value"), ttl)
    value, err := storage.Get(ctx, "key")
    // Works for MemoryStorage, RedisStorage, etc.
}

// Can substitute:
storage1 := memory.New(...)
storage2 := redis.New(...)  // Both implement Storage interface
```

**Principle**: Cache manager works with any storage implementation
**Guarantee**: All implementations follow same contracts

#### 2. Backing Store Swapping
```go
// Cache works identically regardless of backing store
cache := NewBackedCache(storage, redisStore, gossip, cfg)
// OR
cache := NewBackedCache(storage, postgresStore, gossip, cfg)

// Behavior is identical from cache perspective
```

**Principle**: Backing store can be swapped without changing behavior
**Guarantee**: All stores follow same semantics

### Benefits
- **Interchangeability**: Implementations can be swapped easily
- **Testing**: Easy to mock interfaces for testing
- **Flexibility**: Choose implementation at runtime

---

## Interface Segregation Principle (ISP)

> Many client-specific interfaces are better than one general-purpose interface.

### Examples in GossipCache

#### 1. Focused Interfaces

Instead of one large interface:
```go
// BAD: One large interface
type CacheSystem interface {
    // Cache operations
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte) error
    // Gossip operations
    BroadcastChange(ctx context.Context, key string, version int64) error
    // Storage operations
    Evict() error
    // Network operations
    SendMessage(addr string, msg Message) error
}
```

Better design with focused interfaces:
```go
// GOOD: Multiple focused interfaces
type Cache interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte) error
}

type Gossip interface {
    BroadcastChange(ctx context.Context, key string, version int64) error
}

type Storage interface {
    Get(ctx context.Context, key string) (*Entry, error)
    Set(ctx context.Context, key string, value []byte) error
}
```

#### 2. Read vs Write Separation
```go
// Clients only using reads don't need write methods
type CacheReader interface {
    Get(ctx context.Context, key string) ([]byte, error)
    GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)
}

type CacheWriter interface {
    Set(ctx context.Context, key string, value []byte) error
    Delete(ctx context.Context, key string) error
}

// Full interface combines both
type Cache interface {
    CacheReader
    CacheWriter
}
```

### Benefits
- **Minimal Dependencies**: Clients depend only on what they need
- **Clear Contracts**: Each interface has a specific purpose
- **Easier Testing**: Mock only what's needed

---

## Dependency Inversion Principle (DIP)

> Depend upon abstractions, not concretions.

### Examples in GossipCache

#### 1. Cache Manager Dependencies
```go
// cache/backed_cache.go

// BAD: Depend on concrete types
type BackedCache struct {
    storage      *memory.MemoryStorage      // Concrete!
    backingStore *redis.RedisStore          // Concrete!
    gossip       *gossip.ConcreteEngine     // Concrete!
}

// GOOD: Depend on interfaces
type BackedCache struct {
    storage      storage.Storage            // Abstract!
    backingStore backingstore.BackingStore  // Abstract!
    gossip       *gossip.Engine             // Abstract!
}
```

**Benefit**: Can swap implementations without changing BackedCache

#### 2. Dependency Injection
```go
// Good: Inject dependencies via constructor
func NewBackedCache(
    storage storage.Storage,                 // Interface
    backingStore backingstore.BackingStore,  // Interface
    gossip *gossip.Engine,                   // Interface
    config *Config,
) *BackedCache {
    return &BackedCache{
        storage:      storage,
        backingStore: backingStore,
        gossip:       gossip,
        config:       config,
    }
}

// Usage: Caller controls concrete types
cache := NewBackedCache(
    memory.New(...),       // Concrete implementation
    redis.New(...),        // Concrete implementation
    gossip.NewEngine(...), // Concrete implementation
    cfg,
)
```

#### 3. Testing with Mocks
```go
// Easy to test with mocks
func TestBackedCache_Get(t *testing.T) {
    mockStorage := &mock.Storage{}
    mockBackingStore := &mock.BackingStore{}
    mockGossip := &mock.Gossip{}

    cache := NewBackedCache(mockStorage, mockBackingStore, mockGossip, cfg)

    // Test with mocks
    mockStorage.On("Get", "key").Return(nil, errors.New("not found"))
    mockBackingStore.On("Get", "key").Return([]byte("value"), int64(1), nil)

    value, err := cache.Get(ctx, "key")
    // Assertions...
}
```

### Benefits
- **Testability**: Easy to mock dependencies
- **Flexibility**: Swap implementations at runtime
- **Loose Coupling**: Components don't depend on concrete implementations

---

## DRY (Don't Repeat Yourself)

> Every piece of knowledge must have a single, unambiguous representation.

### Examples in GossipCache

#### 1. Singleflight Pattern
```go
// internal/util/singleflight.go
// DRY: One implementation used everywhere

type SingleFlight struct {
    // Implementation once, used in:
    // - Backed cache (prevent thundering herd)
    // - Independent cache (deduplicate requests)
    // - Any other component needing this pattern
}
```

#### 2. Configuration Loading
```go
// internal/config/loader.go
// DRY: Single function loads from file, env, and validates

func Load(path string) (*Config, error) {
    cfg := Default()
    loadFromFile(cfg, path)      // One place
    loadFromEnv(cfg)              // One place
    Validate(cfg)                 // One place
    return cfg, nil
}
```

#### 3. Message Encoding/Decoding
```go
// internal/network/codec.go
// DRY: Centralized encoding logic

func encodeMessage(msg Message) ([]byte, error) {
    // Used by all message types
}

func decodeMessage(data []byte) (Message, error) {
    // Used by all message types
}
```

#### 4. Error Handling
```go
// pkg/gossipcache/errors.go
// DRY: Single source of error definitions

var (
    ErrKeyNotFound   = errors.New("key not found")
    ErrCacheFull     = errors.New("cache full")
    ErrClosed        = errors.New("cache closed")
)

// Used consistently across entire codebase
```

### Benefits
- **Maintainability**: Change once, affects everywhere
- **Consistency**: Same logic everywhere
- **Reduced Bugs**: Fix bug once

---

## Summary

### SOLID Checklist for New Features

When adding new features, ensure:

- [ ] **SRP**: Component has single, well-defined responsibility
- [ ] **OCP**: New functionality via extension, not modification
- [ ] **LSP**: New implementation can substitute existing ones
- [ ] **ISP**: Interfaces are focused and minimal
- [ ] **DIP**: Depend on interfaces, inject dependencies
- [ ] **DRY**: Reuse existing patterns and utilities

### Code Review Guidelines

During code review, verify:

1. **Interfaces**: Are they focused? Can they be split?
2. **Dependencies**: Are they injected? Are they interfaces?
3. **Duplication**: Is there repeated code that can be extracted?
4. **Testability**: Can this be tested with mocks?
5. **Extensibility**: Can new implementations be added easily?

### Package Organization

GossipCache follows these patterns:

```
internal/               # Private implementation
├── cache/             # SRP: Cache coordination
├── storage/           # SRP: Data storage
├── gossip/            # SRP: Gossip protocol
├── backingstore/      # SRP: External store communication
├── network/           # SRP: Network I/O
├── config/            # SRP: Configuration
├── observability/     # SRP: Logging & metrics
└── util/              # DRY: Shared utilities

pkg/                   # Public API
└── gossipcache/       # ISP: Clean client interface
```

Each package has single responsibility and clear boundaries.

---

## References

- [SOLID Principles Explained](https://en.wikipedia.org/wiki/SOLID)
- [Clean Architecture by Robert C. Martin](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Go Proverbs](https://go-proverbs.github.io/)

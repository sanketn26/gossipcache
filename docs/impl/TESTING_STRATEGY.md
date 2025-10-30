# Testing Strategy for GossipCache

Comprehensive testing approach covering unit tests, integration tests, performance tests, and chaos engineering.

## Overview

GossipCache employs a multi-layered testing strategy to ensure correctness, performance, and reliability in distributed scenarios.

## Testing Pyramid

```
                    /\
                   /  \
                  / E2E\         Small number
                 /______\
                /        \
               / Integration\    Medium number
              /______________\
             /                \
            /   Unit Tests     \  Large number
           /____________________\
```

## 1. Unit Tests

**Goal**: Test individual components in isolation

**Coverage Target**: >80%

**Tools**:
- `testing` (Go standard library)
- `github.com/stretchr/testify` (assertions and mocks)
- `github.com/vektra/mockery` (mock generation)

### Unit Test Examples

#### 1.1 Storage Tests
```go
// internal/storage/memory/memory_test.go
func TestMemoryStorage_GetSet(t *testing.T) {
    storage, err := New(1<<20, "lru")
    require.NoError(t, err)
    defer storage.Close()

    ctx := context.Background()

    // Test Set
    err = storage.Set(ctx, "key1", []byte("value1"), 5*time.Minute)
    require.NoError(t, err)

    // Test Get
    entry, err := storage.Get(ctx, "key1")
    require.NoError(t, err)
    assert.Equal(t, []byte("value1"), entry.Value)
}

func TestMemoryStorage_Expiration(t *testing.T) {
    storage, _ := New(1<<20, "lru")
    defer storage.Close()

    ctx := context.Background()

    // Set with short TTL
    storage.Set(ctx, "key1", []byte("value1"), 100*time.Millisecond)

    // Should exist
    _, err := storage.Get(ctx, "key1")
    require.NoError(t, err)

    // Wait for expiration
    time.Sleep(150 * time.Millisecond)

    // Should be expired
    _, err = storage.Get(ctx, "key1")
    assert.Error(t, err)
}
```

#### 1.2 Vector Clock Tests
```go
// internal/vclock/vclock_test.go
func TestVectorClock_Compare(t *testing.T) {
    tests := []struct {
        name     string
        local    VectorClock
        remote   VectorClock
        expected Relation
    }{
        {
            name:     "equal clocks",
            local:    VectorClock{"n1": 5, "n2": 3},
            remote:   VectorClock{"n1": 5, "n2": 3},
            expected: Equal,
        },
        {
            name:     "local newer",
            local:    VectorClock{"n1": 6, "n2": 3},
            remote:   VectorClock{"n1": 5, "n2": 3},
            expected: LocalNewer,
        },
        {
            name:     "concurrent",
            local:    VectorClock{"n1": 6, "n2": 2},
            remote:   VectorClock{"n1": 5, "n2": 3},
            expected: Concurrent,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := Compare(tt.local, tt.remote)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

#### 1.3 Mock-Based Tests
```go
// internal/cache/backed_cache_test.go
func TestBackedCache_Get_CacheMiss(t *testing.T) {
    mockStorage := &mock.Storage{}
    mockBackingStore := &mock.BackingStore{}
    mockGossip := &mock.Gossip{}

    cache := NewBackedCache(mockStorage, mockBackingStore, mockGossip, cfg)

    ctx := context.Background()

    // Setup mocks
    mockStorage.On("Get", ctx, "key1").Return(nil, storage.ErrKeyNotFound)
    mockBackingStore.On("Get", ctx, "key1").Return([]byte("value1"), int64(1), nil)
    mockStorage.On("Set", ctx, "key1", []byte("value1"), mock.Anything).Return(nil)

    // Test
    value, err := cache.Get(ctx, "key1")
    require.NoError(t, err)
    assert.Equal(t, []byte("value1"), value)

    // Verify mocks
    mockStorage.AssertExpectations(t)
    mockBackingStore.AssertExpectations(t)
}
```

### Running Unit Tests

```bash
# Run all unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package
go test ./internal/storage/memory/

# Run with race detector
go test -race ./...

# Verbose output
go test -v ./...
```

---

## 2. Integration Tests

**Goal**: Test component interactions with real dependencies

**Location**: `test/integration/`

**Requirements**:
- Docker for running Redis/Postgres
- Network access
- Longer timeouts

### Integration Test Examples

#### 2.1 Backed Mode with Redis
```go
// test/integration/backed_mode_test.go
func TestBackedMode_ThreeNodeCluster(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Start Redis via testcontainers
    redis := startRedisContainer(t)
    defer redis.Stop()

    // Setup 3-node cluster
    nodes := make([]*BackedCache, 3)
    for i := 0; i < 3; i++ {
        nodes[i] = setupNode(t, redis.Address(), 7946+i)
    }
    defer cleanupNodes(nodes)

    // Connect nodes as peers
    for i, node := range nodes {
        for j, peer := range nodes {
            if i != j {
                node.AddPeer(peer.NodeID(), peer.Address())
            }
        }
    }

    ctx := context.Background()

    // Write on node 0
    err := nodes[0].Set(ctx, "test_key", []byte("test_value"), 5*time.Minute)
    require.NoError(t, err)

    // Wait for gossip propagation
    time.Sleep(2 * time.Second)

    // Read from node 1 (should get from Redis)
    value1, err := nodes[1].Get(ctx, "test_key")
    require.NoError(t, err)
    assert.Equal(t, []byte("test_value"), value1)

    // Read from node 2 (should get from local cache or Redis)
    value2, err := nodes[2].Get(ctx, "test_key")
    require.NoError(t, err)
    assert.Equal(t, []byte("test_value"), value2)

    // Verify stats show hits/misses
    stats1, _ := nodes[1].Stats(ctx)
    t.Logf("Node 1 stats: hits=%d, misses=%d", stats1.Hits, stats1.Misses)
}
```

#### 2.2 Independent Mode with Conflicts
```go
// test/integration/independent_mode_test.go
func TestIndependentMode_ConflictResolution(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Setup 3-node cluster
    nodes := setupIndependentCluster(t, 3)
    defer cleanupNodes(nodes)

    ctx := context.Background()

    // Partition network: isolate node 0 from nodes 1,2
    partition := NewNetworkPartition()
    partition.Isolate(nodes[0], []Cache{nodes[1], nodes[2]})

    // Concurrent writes on different partitions
    err1 := nodes[0].Set(ctx, "key1", []byte("value_from_node0"), 0)
    require.NoError(t, err1)

    err2 := nodes[1].Set(ctx, "key1", []byte("value_from_node1"), 0)
    require.NoError(t, err2)

    // Heal partition
    partition.Heal()

    // Wait for gossip and conflict resolution
    time.Sleep(5 * time.Second)

    // All nodes should converge to same value (LWW)
    values := make(map[int][]byte)
    for i, node := range nodes {
        val, err := node.Get(ctx, "key1")
        require.NoError(t, err)
        values[i] = val
        t.Logf("Node %d value: %s", i, val)
    }

    // Verify convergence
    assert.Equal(t, values[0], values[1])
    assert.Equal(t, values[1], values[2])
}
```

### Running Integration Tests

```bash
# Run only short tests (skip integration)
go test -short ./...

# Run integration tests
go test ./test/integration/

# Run with Docker Compose
docker-compose -f test/docker-compose.yml up -d
go test ./test/integration/
docker-compose -f test/docker-compose.yml down
```

---

## 3. Benchmark Tests

**Goal**: Measure performance and identify bottlenecks

**Location**: `test/benchmark/`

### Benchmark Examples

```go
// test/benchmark/cache_bench_test.go
func BenchmarkCache_Get_Hit(b *testing.B) {
    cache := setupCache(b)
    ctx := context.Background()

    // Pre-populate
    cache.Set(ctx, "key1", []byte("value1"), 5*time.Minute)

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        cache.Get(ctx, "key1")
    }
}

func BenchmarkCache_Set(b *testing.B) {
    cache := setupCache(b)
    ctx := context.Background()
    value := []byte("test_value")

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        key := fmt.Sprintf("key%d", i)
        cache.Set(ctx, key, value, 5*time.Minute)
    }
}

func BenchmarkCache_Concurrent(b *testing.B) {
    cache := setupCache(b)
    ctx := context.Background()

    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            key := fmt.Sprintf("key%d", rand.Intn(1000))
            cache.Get(ctx, key)
        }
    })
}
```

### Running Benchmarks

```bash
# Run benchmarks
go test -bench=. ./test/benchmark/

# With memory allocations
go test -bench=. -benchmem ./test/benchmark/

# Compare results
go test -bench=. -benchmem ./test/benchmark/ > old.txt
# Make changes...
go test -bench=. -benchmem ./test/benchmark/ > new.txt
benchcmp old.txt new.txt
```

---

## 4. Chaos Testing

**Goal**: Test system behavior under adverse conditions

**Location**: `test/chaos/`

### Chaos Test Examples

```go
// test/chaos/partition_test.go
func TestChaos_RandomPartitions(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping chaos test")
    }

    nodes := setupCluster(t, 5)
    defer cleanupNodes(nodes)

    // Run for 5 minutes with random partitions
    duration := 5 * time.Minute
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    ctx, cancel := context.WithTimeout(context.Background(), duration)
    defer cancel()

    operations := 0

    for {
        select {
        case <-ctx.Done():
            t.Logf("Chaos test complete. Operations: %d", operations)
            verifyEventualConsistency(t, nodes)
            return

        case <-ticker.C:
            // Create random partition
            partition := createRandomPartition(nodes)
            t.Logf("Created partition: %v", partition)

            // Perform operations
            ops := performRandomOperations(t, nodes, 100)
            operations += ops

            // Heal after some time
            time.Sleep(15 * time.Second)
            partition.Heal()
            t.Logf("Healed partition")
        }
    }
}

func TestChaos_NodeFailures(t *testing.T) {
    nodes := setupCluster(t, 5)
    defer cleanupNodes(nodes)

    ctx := context.Background()

    // Kill random nodes
    for i := 0; i < 10; i++ {
        // Random operations
        performRandomOperations(t, nodes, 50)

        // Kill random node
        victim := rand.Intn(len(nodes))
        nodes[victim].Close()
        t.Logf("Killed node %d", victim)

        // Continue with remaining nodes
        time.Sleep(5 * time.Second)

        // Restart node
        nodes[victim] = restartNode(t, victim)
        t.Logf("Restarted node %d", victim)

        time.Sleep(5 * time.Second)
    }

    // Verify eventual consistency
    verifyEventualConsistency(t, nodes)
}
```

---

## 5. Load Testing

**Goal**: Test system under realistic load

**Tools**:
- [vegeta](https://github.com/tsenart/vegeta)
- [k6](https://k6.io/)
- Custom Go load tests

### Load Test Examples

```bash
# Vegeta load test
echo "GET http://localhost:8080/api/v1/cache/test" | \
  vegeta attack -duration=60s -rate=10000/s | \
  vegeta report

# With multiple endpoints
cat targets.txt | vegeta attack -duration=60s -rate=5000/s | vegeta report

# targets.txt:
# GET http://localhost:8080/api/v1/cache/key1
# GET http://localhost:8080/api/v1/cache/key2
# POST http://localhost:8080/api/v1/cache/key3
# @body.json
```

```go
// test/load/load_test.go
func TestLoad_SustainedTraffic(t *testing.T) {
    cache := setupCache(t)
    ctx := context.Background()

    // Simulate 10K req/s for 60 seconds
    duration := 60 * time.Second
    rps := 10000
    interval := time.Second / time.Duration(rps)

    start := time.Now()
    requests := 0
    errors := 0

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    timeout := time.After(duration)

    for {
        select {
        case <-timeout:
            elapsed := time.Since(start)
            actualRPS := float64(requests) / elapsed.Seconds()
            errorRate := float64(errors) / float64(requests) * 100

            t.Logf("Load test complete:")
            t.Logf("  Duration: %v", elapsed)
            t.Logf("  Requests: %d", requests)
            t.Logf("  Actual RPS: %.2f", actualRPS)
            t.Logf("  Errors: %d (%.2f%%)", errors, errorRate)

            assert.Less(t, errorRate, 1.0, "Error rate too high")
            return

        case <-ticker.C:
            go func() {
                key := fmt.Sprintf("key%d", rand.Intn(10000))
                _, err := cache.Get(ctx, key)
                if err != nil {
                    atomic.AddInt64(&errors, 1)
                }
            }()
            requests++
        }
    }
}
```

---

## 6. Property-Based Testing

**Goal**: Generate random inputs to find edge cases

**Tool**: [gopter](https://github.com/leanovate/gopter)

```go
// internal/vclock/vclock_property_test.go
func TestVectorClock_Properties(t *testing.T) {
    properties := gopter.NewProperties(nil)

    // Property: Merging is commutative
    properties.Property("merge is commutative", prop.ForAll(
        func(a, b VectorClock) bool {
            merge1 := Merge(a, b)
            merge2 := Merge(b, a)
            return reflect.DeepEqual(merge1, merge2)
        },
        genVectorClock(),
        genVectorClock(),
    ))

    // Property: Merging is idempotent
    properties.Property("merge is idempotent", prop.ForAll(
        func(a VectorClock) bool {
            merge1 := Merge(a, a)
            return reflect.DeepEqual(a, merge1)
        },
        genVectorClock(),
    ))

    properties.TestingRun(t)
}
```

---

## 7. Test Organization

### Directory Structure
```
gossipcache/
├── internal/
│   └── */
│       └── *_test.go          # Unit tests alongside code
├── test/
│   ├── integration/           # Integration tests
│   │   ├── backed_mode_test.go
│   │   └── independent_mode_test.go
│   ├── benchmark/             # Benchmarks
│   │   └── cache_bench_test.go
│   ├── chaos/                 # Chaos tests
│   │   └── partition_test.go
│   ├── load/                  # Load tests
│   │   └── load_test.go
│   └── helpers/               # Test utilities
│       ├── cluster.go         # Cluster setup
│       └── network.go         # Network simulation
```

### Test Tags

```go
// +build integration
// test/integration/backed_mode_test.go

// +build chaos
// test/chaos/partition_test.go
```

```bash
# Run only unit tests
go test -short ./...

# Run integration tests
go test -tags=integration ./test/integration/

# Run chaos tests
go test -tags=chaos ./test/chaos/
```

---

## 8. CI/CD Pipeline

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: 1.21
      - name: Unit Tests
        run: go test -short -race -coverprofile=coverage.out ./...
      - name: Upload Coverage
        uses: codecov/codecov-action@v2

  integration:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
      - name: Integration Tests
        run: go test ./test/integration/

  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
      - name: Benchmarks
        run: go test -bench=. -benchmem ./test/benchmark/
```

---

## 9. Testing Checklist

Before merging code, ensure:

- [ ] Unit tests written for new code
- [ ] Test coverage >80%
- [ ] Integration tests pass
- [ ] Benchmarks show acceptable performance
- [ ] Race detector passes (`go test -race`)
- [ ] No flaky tests
- [ ] Mocks are up-to-date
- [ ] Tests are documented

---

## 10. Performance Targets

| Metric | Target | Test |
|--------|--------|------|
| Get (hit) latency | < 1ms | Benchmark |
| Set latency | < 2ms | Benchmark |
| Throughput | > 100K ops/s | Load test |
| Gossip propagation | < 500ms | Integration |
| Memory overhead | < 10% of data | Benchmark |
| Recovery time | < 30s | Chaos test |

---

## References

- [Go Testing](https://golang.org/pkg/testing/)
- [Testify](https://github.com/stretchr/testify)
- [Table-Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [Advanced Testing in Go](https://www.youtube.com/watch?v=8hQG7QlcLBk)

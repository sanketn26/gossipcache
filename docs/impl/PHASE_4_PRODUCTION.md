# Phase 4: Production Readiness

**Goal**: Add production features including multiple backing stores, deployment support, monitoring, and operational tooling.

**Duration**: 2-3 weeks

**Prerequisites**: Phases 1-3 complete

**Status**: Not Started

## Overview

Phase 4 makes GossipCache production-ready with support for additional backing stores (Postgres, MySQL), node discovery for various environments (EC2, Docker, K8s), comprehensive monitoring, and production deployment manifests.

## Objectives

- [ ] Postgres backing store connector
- [ ] MySQL backing store connector
- [ ] EC2 node discovery
- [ ] Docker node discovery
- [ ] Kubernetes node discovery
- [ ] **DNS-based discovery** (NEW)
- [ ] HTTP API for management
- [ ] **Debug endpoints** (/debug/peers, /debug/gossip) (NEW)
- [ ] **pprof endpoints** (/debug/pprof/*) (NEW)
- [ ] Prometheus metrics
- [ ] Health checks and readiness probes
- [ ] Graceful shutdown
- [ ] Deployment manifests (Docker Compose, K8s)
- [ ] Load testing and performance tuning
- [ ] Documentation and runbooks

## Implementation Steps

### Step 1: Postgres Backing Store (Day 1-4)

```go
// internal/backingstore/postgres/postgres.go
package postgres

import (
    "context"
    "database/sql"
    "fmt"

    _ "github.com/lib/pq"
    "github.com/yourorg/gossipcache/internal/backingstore"
)

type PostgresStore struct {
    db *sql.DB
}

func New(cfg *backingstore.Config) (*PostgresStore, error) {
    connStr := fmt.Sprintf(
        "host=%s user=%s password=%s dbname=%s sslmode=disable",
        cfg.Address, cfg.Username, cfg.Password, cfg.Database,
    )

    db, err := sql.Open("postgres", connStr)
    if err != nil {
        return nil, err
    }

    // Create schema
    if err := createSchema(db); err != nil {
        return nil, err
    }

    return &PostgresStore{db: db}, nil
}

func createSchema(db *sql.DB) error {
    schema := `
        CREATE TABLE IF NOT EXISTS cache_entries (
            key VARCHAR(1024) PRIMARY KEY,
            value BYTEA NOT NULL,
            version BIGSERIAL NOT NULL,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
    `
    _, err := db.Exec(schema)
    return err
}

func (p *PostgresStore) Get(ctx context.Context, key string) ([]byte, int64, error) {
    var value []byte
    var version int64

    err := p.db.QueryRowContext(ctx,
        "SELECT value, version FROM cache_entries WHERE key = $1",
        key,
    ).Scan(&value, &version)

    if err == sql.ErrNoRows {
        return nil, 0, backingstore.ErrKeyNotFound
    }

    return value, version, err
}

func (p *PostgresStore) Set(ctx context.Context, key string, value []byte) (int64, error) {
    var version int64

    // Upsert with version increment
    err := p.db.QueryRowContext(ctx, `
        INSERT INTO cache_entries (key, value, version)
        VALUES ($1, $2, 1)
        ON CONFLICT (key) DO UPDATE
        SET value = EXCLUDED.value,
            version = cache_entries.version + 1,
            updated_at = CURRENT_TIMESTAMP
        RETURNING version
    `, key, value).Scan(&version)

    return version, err
}

// Implement other methods...
```

### Step 2: Node Discovery (Day 5-9)

```go
// internal/discovery/discovery.go
package discovery

type Discovery interface {
    Discover(ctx context.Context) ([]NodeInfo, error)
    Register(ctx context.Context, self NodeInfo) error
}

type NodeInfo struct {
    NodeID  string
    Address string
}

func New(cfg *Config) (Discovery, error) {
    switch cfg.Mode {
    case "static":
        return NewStaticDiscovery(cfg.Peers), nil
    case "ec2":
        return NewEC2Discovery(cfg.EC2), nil
    case "docker":
        return NewDockerDiscovery(), nil
    case "kubernetes":
        return NewKubernetesDiscovery(cfg.Kubernetes), nil
    default:
        return nil, fmt.Errorf("unsupported discovery mode: %s", cfg.Mode)
    }
}
```

### Step 3: HTTP API (Day 10-11)

```go
// internal/api/server.go
package api

type Server struct {
    cache  gossipcache.Cache
    router *http.ServeMux
}

func NewServer(cache gossipcache.Cache, port int) *Server {
    s := &Server{
        cache:  cache,
        router: http.NewServeMux(),
    }

    s.registerRoutes()
    return s
}

func (s *Server) registerRoutes() {
    s.router.HandleFunc("/api/v1/cache/", s.handleCache)
    s.router.HandleFunc("/api/v1/stats", s.handleStats)
    s.router.HandleFunc("/api/v1/peers", s.handlePeers)
    s.router.HandleFunc("/health", s.handleHealth)
    s.router.HandleFunc("/ready", s.handleReady)
}
```

### Step 4: Prometheus Metrics (Day 12-13)

```go
// internal/observability/metrics.go
package observability

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
    cacheHits      *prometheus.CounterVec
    cacheMisses    *prometheus.CounterVec
    cacheEvictions *prometheus.CounterVec
    gossipMessages *prometheus.CounterVec
    peerCount      prometheus.Gauge
}

func NewMetrics() *Metrics {
    m := &Metrics{
        cacheHits: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "gossipcache_hits_total",
                Help: "Total number of cache hits",
            },
            []string{"node_id"},
        ),
        // ... other metrics
    }

    prometheus.MustRegister(m.cacheHits, m.cacheMisses, ...)

    return m
}
```

### Step 5: Graceful Shutdown (Day 13-14)

```go
// cmd/gossipcache/main.go
func main() {
    // ... setup ...

    // Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

    <-sigCh
    logger.Info("shutting down gracefully")

    // Create shutdown context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Stop gossip engine
    if err := gossipEngine.Stop(); err != nil {
        logger.Error("error stopping gossip", "error", err)
    }

    // Close cache
    if err := cache.Close(); err != nil {
        logger.Error("error closing cache", "error", err)
    }

    logger.Info("shutdown complete")
}
```

### Step 6: Deployment Manifests (Day 15-17)

Create production-ready manifests:
- Docker Compose files
- Kubernetes StatefulSets
- Helm charts (optional)
- Terraform modules (optional)

### Step 7: Load Testing (Day 18-20)

```bash
# Load test with vegeta
echo "GET http://localhost:8080/api/v1/cache/test" | \
  vegeta attack -duration=60s -rate=10000 | \
  vegeta report
```

### Step 8: Documentation (Day 21)

- Update README with quick start
- API documentation
- Deployment guides
- Operational runbooks
- Troubleshooting guides

## Deliverables

- [ ] Multiple backing store support (Redis, Postgres)
- [ ] Node discovery for all environments
- [ ] HTTP API
- [ ] Prometheus metrics
- [ ] Deployment manifests
- [ ] Load test results
- [ ] Complete documentation

## Success Criteria

1. **Production-Ready**: Can be deployed to production environments
2. **Observable**: Metrics, logging, health checks
3. **Scalable**: Load tested to target throughput
4. **Documented**: Complete docs and runbooks

## Post-Phase 4

At this point, GossipCache is feature-complete and production-ready. Future work can include:
- Additional features (query support, compression, etc.)
- Performance optimizations
- Additional backing stores
- Multi-region support

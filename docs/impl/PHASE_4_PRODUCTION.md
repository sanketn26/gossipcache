# Phase 4: Production Readiness

**Goal**: Add production features including multiple backing stores, deployment support, monitoring, and operational tooling.

**Duration**: 2.5-3.5 weeks

**Prerequisites**: Phases 1-3 complete

**Status**: Not Started

## Overview

Phase 4 makes GossipCache production-ready with support for additional backing stores (Postgres, MySQL), node discovery for various environments (EC2, Docker, Kubernetes, DNS), comprehensive monitoring, debug tooling, and production deployment manifests.

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

### Phase 4 TDD Rhythm

Production features should start with contract tests and failure-mode tests. For external systems, keep fast unit tests around adapters and add tagged integration tests for real Postgres, discovery providers, HTTP behavior, metrics, and shutdown.

### Step 1: SQL Backing Stores (Day 1-5)

Implement Postgres first, then reuse the SQL patterns for MySQL/MariaDB. Both stores must satisfy `backingstore.BackingStore`, use connection pooling through `database/sql`, create their schema on startup, and maintain monotonically increasing per-key versions.

```go
// internal/backingstore/postgres/postgres.go
package postgres

import (
    "context"
    "database/sql"
    "fmt"

    _ "github.com/lib/pq"
    "github.com/sanketn26/gossipcache/internal/backingstore"
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

```go
// internal/backingstore/mysql/mysql.go
package mysql

import (
    "context"
    "database/sql"
    "fmt"

    _ "github.com/go-sql-driver/mysql"
    "github.com/sanketn26/gossipcache/internal/backingstore"
)

type MySQLStore struct {
    db *sql.DB
}

func New(cfg *backingstore.Config) (*MySQLStore, error) {
    dsn := fmt.Sprintf(
        "%s:%s@tcp(%s)/%s?parseTime=true",
        cfg.Username, cfg.Password, cfg.Address, cfg.Database,
    )

    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return nil, err
    }

    if err := createSchema(db); err != nil {
        return nil, err
    }

    return &MySQLStore{db: db}, nil
}

func createSchema(db *sql.DB) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS cache_entries (
            ` + "`key`" + ` VARCHAR(1024) PRIMARY KEY,
            ` + "`value`" + ` BLOB NOT NULL,
            ` + "`version`" + ` BIGINT NOT NULL DEFAULT 1,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            INDEX version_idx (` + "`version`" + `)
        ) ENGINE=InnoDB;
    `)
    return err
}

func (m *MySQLStore) Set(ctx context.Context, key string, value []byte) (int64, error) {
    _, err := m.db.ExecContext(ctx, `
        INSERT INTO cache_entries (` + "`key`" + `, ` + "`value`" + `, ` + "`version`" + `)
        VALUES (?, ?, 1)
        ON DUPLICATE KEY UPDATE
            ` + "`value`" + ` = VALUES(` + "`value`" + `),
            ` + "`version`" + ` = ` + "`version`" + ` + 1,
            updated_at = CURRENT_TIMESTAMP
    `, key, value)
    if err != nil {
        return 0, err
    }

    var version int64
    err = m.db.QueryRowContext(ctx,
        "SELECT `version` FROM cache_entries WHERE `key` = ?",
        key,
    ).Scan(&version)
    return version, err
}
```

### Step 2: Node Discovery (Day 6-10)

```go
// internal/discovery/discovery.go
package discovery

import (
    "context"
    "fmt"
)

type Discovery interface {
    Discover(ctx context.Context) ([]NodeInfo, error)
    Register(ctx context.Context, self NodeInfo) error
    Deregister(ctx context.Context, nodeID string) error
    Watch(ctx context.Context) (<-chan []NodeInfo, error)
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
    case "dns":
        return NewDNSDiscovery(cfg.DNS), nil
    default:
        return nil, fmt.Errorf("unsupported discovery mode: %s", cfg.Mode)
    }
}
```

#### DNS Discovery

DNS discovery is required for Kubernetes headless services and other environments where SRV or A/AAAA records are the simplest stable peer source.

```go
// internal/discovery/dns_discovery.go
package discovery

import (
    "context"
    "fmt"
    "net"
    "time"
)

type DNSConfig struct {
    ServiceName string
    Domain      string
    Port        int
    Interval    time.Duration
}

type DNSDiscovery struct {
    cfg DNSConfig
}

func NewDNSDiscovery(cfg DNSConfig) *DNSDiscovery {
    return &DNSDiscovery{cfg: cfg}
}

func (d *DNSDiscovery) Discover(ctx context.Context) ([]NodeInfo, error) {
    _, records, err := net.LookupSRV(d.cfg.ServiceName, "tcp", d.cfg.Domain)
    if err == nil {
        nodes := make([]NodeInfo, 0, len(records))
        for _, record := range records {
            nodes = append(nodes, NodeInfo{
                NodeID:  fmt.Sprintf("%s:%d", record.Target, record.Port),
                Address: fmt.Sprintf("%s:%d", record.Target, record.Port),
            })
        }
        return nodes, nil
    }

    host := fmt.Sprintf("%s.%s", d.cfg.ServiceName, d.cfg.Domain)
    ips, err := net.LookupIP(host)
    if err != nil {
        return nil, err
    }

    nodes := make([]NodeInfo, 0, len(ips))
    for _, ip := range ips {
        nodes = append(nodes, NodeInfo{
            NodeID:  fmt.Sprintf("%s:%d", ip.String(), d.cfg.Port),
            Address: fmt.Sprintf("%s:%d", ip.String(), d.cfg.Port),
        })
    }
    return nodes, nil
}
```

### Step 3: HTTP API and Debug Endpoints (Day 11-12)

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
    s.registerDebugRoutes()
    s.registerPprofRoutes()
}

func (s *Server) registerDebugRoutes() {
    s.router.HandleFunc("/debug/peers", s.handleDebugPeers)
    s.router.HandleFunc("/debug/gossip", s.handleDebugGossip)
    s.router.HandleFunc("/debug/cache", s.handleDebugCacheSample)
}
```

Debug and pprof endpoints must be gated by configuration and disabled by default in internet-facing deployments.

```go
// internal/api/debug.go
package api

import "net/http/pprof"

func (s *Server) registerPprofRoutes() {
    s.router.HandleFunc("/debug/pprof/", pprof.Index)
    s.router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
    s.router.HandleFunc("/debug/pprof/profile", pprof.Profile)
    s.router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
    s.router.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
```

### Step 4: Prometheus Metrics (Day 13-14)

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
    gossipDropped  *prometheus.CounterVec
    gossipQueueLen prometheus.Gauge
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

### Step 5: Graceful Shutdown (Day 14-15)

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

### Step 6: Deployment Manifests (Day 16-18)

Create production-ready manifests:
- Docker Compose files
- Kubernetes StatefulSets
- Helm charts (optional)
- Terraform modules (optional)

### Step 7: Load Testing (Day 19-21)

```bash
# Load test with vegeta
echo "GET http://localhost:8080/api/v1/cache/test" | \
  vegeta attack -duration=60s -rate=10000 | \
  vegeta report
```

### Step 8: Documentation and Runbooks (Day 22-23)

- Update README with quick start
- API documentation
- Deployment guides
- Operational runbooks
- Troubleshooting guides
- Rolling update strategy
- Independent-mode backup/restore limitations and future plan

## Deliverables

- [ ] Multiple backing store support (Redis, Postgres, MySQL)
- [ ] Node discovery for static, EC2, Docker, Kubernetes API, and DNS
- [ ] HTTP API
- [ ] Debug and pprof endpoints
- [ ] Prometheus metrics, including gossip queue and drop counters
- [ ] Deployment manifests
- [ ] Load test results
- [ ] Complete documentation and runbooks

## TDD Test Plan

| Slice | Write This Test First | Expected Behavior | Checkpoint |
| --- | --- | --- | --- |
| Postgres store contract | `internal/backingstore/postgres/postgres_test.go` | SQL adapter maps rows, versions, not-found, and transient errors correctly through fakes | `go test ./internal/backingstore/postgres` |
| Postgres integration | `internal/backingstore/postgres/postgres_integration_test.go` | Real Postgres round-trips data and increments versions transactionally | `go test -tags=integration ./internal/backingstore/postgres` |
| Discovery contract | `internal/discovery/discovery_test.go` | Providers return stable peer sets, ignore self, and surface resolver errors | `go test ./internal/discovery` |
| Kubernetes discovery | `internal/discovery/kubernetes_test.go` | Pod/service fixtures become peer addresses with deterministic filtering | `go test ./internal/discovery` |
| HTTP API | `internal/api/http_test.go` | Get/Set/Delete/Stats map HTTP status codes to cache outcomes | `go test ./internal/api` |
| Debug endpoints | `internal/api/debug_test.go` | Debug endpoints are disabled by default and require explicit enablement | `go test ./internal/api` |
| Metrics | `internal/observability/metrics_test.go` | Prometheus collectors expose cache, gossip, and backing store metrics without duplicate registration | `go test ./internal/observability` |
| Health checks | `internal/api/health_test.go` | Liveness and readiness reflect cache, backing store, and gossip state | `go test ./internal/api` |
| Graceful shutdown | `internal/server/shutdown_test.go` | Shutdown drains requests, closes cache resources, and honors context deadlines | `go test -race ./internal/server` |
| Manifests | `test/deploy/manifests_test.go` | Rendered manifests include required ports, probes, resources, and config references | `go test ./test/deploy` |
| Load smoke | `test/load/load_test.go` | Sustained read/write traffic meets error-rate and latency thresholds | `go test -tags=load ./test/load` |

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
- Security hardening from [Phase 4.5 Security](PHASE_4_5_SECURITY.md)

# Phase 4 Addendum: Additional Production Features

**Based on**: Gap Analysis findings
**Adds to**: PHASE_4_PRODUCTION.md

This addendum covers high-priority features identified in the gap analysis that should be added to Phase 4.

## Additional Steps

### Step 1.5: MySQL Backing Store (Day 4-5)

Similar to Postgres but for MySQL/MariaDB.

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

    // Create schema
    if err := createSchema(db); err != nil {
        return nil, err
    }

    return &MySQLStore{db: db}, nil
}

func createSchema(db *sql.DB) error {
    schema := `
        CREATE TABLE IF NOT EXISTS cache_entries (
            ` + "`key`" + ` VARCHAR(1024) PRIMARY KEY,
            ` + "`value`" + ` BLOB NOT NULL,
            ` + "`version`" + ` BIGINT NOT NULL AUTO_INCREMENT,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            INDEX version_idx (` + "`version`" + `)
        ) ENGINE=InnoDB;
    `
    _, err := db.Exec(schema)
    return err
}

func (m *MySQLStore) Set(ctx context.Context, key string, value []byte) (int64, error) {
    // Use INSERT ... ON DUPLICATE KEY UPDATE for upsert
    result, err := m.db.ExecContext(ctx, `
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

    // Get the new version
    var version int64
    err = m.db.QueryRowContext(ctx,
        "SELECT `version` FROM cache_entries WHERE `key` = ?",
        key,
    ).Scan(&version)

    return version, err
}

// Implement Get, Delete, GetMulti, SetMulti, Ping, Close...
```

**Testing**:
```go
func TestMySQLStore_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping MySQL integration test")
    }

    // Use testcontainers for MySQL
    mysqlC := startMySQLContainer(t)
    defer mysqlC.Stop()

    store, err := New(&backingstore.Config{
        Address:  mysqlC.Address(),
        Username: "root",
        Password: "password",
        Database: "testdb",
    })
    require.NoError(t, err)
    defer store.Close()

    // Test operations...
}
```

---

### Step 2.5: DNS-Based Discovery (Day 9-10)

For Kubernetes headless services and SRV records.

```go
// internal/discovery/dns_discovery.go
package discovery

import (
    "context"
    "fmt"
    "net"
    "time"
)

// DNSDiscovery discovers nodes via DNS SRV records
type DNSDiscovery struct {
    serviceName string
    domain      string
    port        int
    interval    time.Duration
}

type DNSConfig struct {
    ServiceName string // e.g., "gossipcache"
    Domain      string // e.g., "default.svc.cluster.local"
    Port        int
    Interval    time.Duration
}

func NewDNSDiscovery(cfg *DNSConfig) *DNSDiscovery {
    return &DNSDiscovery{
        serviceName: cfg.ServiceName,
        domain:      cfg.Domain,
        port:        cfg.Port,
        interval:    cfg.Interval,
    }
}

func (d *DNSDiscovery) Discover(ctx context.Context) ([]NodeInfo, error) {
    // Lookup SRV records
    _, addrs, err := net.LookupSRV(d.serviceName, "tcp", d.domain)
    if err != nil {
        // Fallback to A/AAAA records
        return d.discoverARecords(ctx)
    }

    nodes := make([]NodeInfo, 0, len(addrs))

    for _, addr := range addrs {
        // Resolve target to IP
        ips, err := net.LookupIP(addr.Target)
        if err != nil {
            continue
        }

        for _, ip := range ips {
            nodes = append(nodes, NodeInfo{
                NodeID:  fmt.Sprintf("%s:%d", ip.String(), addr.Port),
                Address: fmt.Sprintf("%s:%d", ip.String(), addr.Port),
            })
        }
    }

    return nodes, nil
}

func (d *DNSDiscovery) discoverARecords(ctx context.Context) ([]NodeInfo, error) {
    // Fallback: lookup A/AAAA records
    hostname := fmt.Sprintf("%s.%s", d.serviceName, d.domain)

    ips, err := net.LookupIP(hostname)
    if err != nil {
        return nil, err
    }

    nodes := make([]NodeInfo, 0, len(ips))

    for _, ip := range ips {
        nodes = append(nodes, NodeInfo{
            NodeID:  fmt.Sprintf("%s:%d", ip.String(), d.port),
            Address: fmt.Sprintf("%s:%d", ip.String(), d.port),
        })
    }

    return nodes, nil
}

func (d *DNSDiscovery) Watch(ctx context.Context) (<-chan []NodeInfo, error) {
    ch := make(chan []NodeInfo)

    go func() {
        ticker := time.NewTicker(d.interval)
        defer ticker.Stop()
        defer close(ch)

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                nodes, err := d.Discover(ctx)
                if err == nil {
                    ch <- nodes
                }
            }
        }
    }()

    return ch, nil
}
```

**Kubernetes Headless Service Configuration**:
```yaml
# k8s/service-headless.yaml
apiVersion: v1
kind: Service
metadata:
  name: gossipcache
  namespace: default
spec:
  clusterIP: None  # Headless service
  selector:
    app: gossipcache
  ports:
  - name: gossip
    port: 7946
    targetPort: 7946
```

**Usage**:
```go
discovery := dns.NewDNSDiscovery(&dns.DNSConfig{
    ServiceName: "gossipcache",
    Domain:      "default.svc.cluster.local",
    Port:        7946,
    Interval:    30 * time.Second,
})

nodes, err := discovery.Discover(ctx)
```

---

### Step 3.5: Debug and pprof Endpoints (Day 11-12)

Add operational debugging endpoints.

```go
// internal/api/debug.go
package api

import (
    "encoding/json"
    "net/http"
    "net/http/pprof"
)

func (s *Server) registerDebugRoutes() {
    // pprof endpoints
    s.router.HandleFunc("/debug/pprof/", pprof.Index)
    s.router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
    s.router.HandleFunc("/debug/pprof/profile", pprof.Profile)
    s.router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
    s.router.HandleFunc("/debug/pprof/trace", pprof.Trace)

    // Custom debug endpoints
    s.router.HandleFunc("/debug/peers", s.handleDebugPeers)
    s.router.HandleFunc("/debug/gossip", s.handleDebugGossip)
    s.router.HandleFunc("/debug/cache", s.handleDebugCache)
}

func (s *Server) handleDebugPeers(w http.ResponseWriter, r *http.Request) {
    peers := s.gossipEngine.GetPeers()

    response := map[string]interface{}{
        "count": len(peers),
        "peers": peers,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

func (s *Server) handleDebugGossip(w http.ResponseWriter, r *http.Request) {
    stats := s.gossipEngine.Stats()

    response := map[string]interface{}{
        "messages_sent":     stats.MessagesSent,
        "messages_received": stats.MessagesReceived,
        "queue_depth":       stats.QueueDepth,
        "dropped_messages":  stats.DroppedMessages,
        "last_gossip":       stats.LastGossipTime,
        "anti_entropy":      stats.LastAntiEntropyTime,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

func (s *Server) handleDebugCache(w http.ResponseWriter, r *http.Request) {
    // Sample cache contents (limit to 100 keys for safety)
    ctx := r.Context()

    keys, err := s.cache.Keys(ctx, 100)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    response := map[string]interface{}{
        "sample_size": len(keys),
        "keys":        keys,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}
```

**Usage**:
```bash
# CPU profiling
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof

# Heap profiling
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Debug endpoints
curl http://localhost:8080/debug/peers | jq
curl http://localhost:8080/debug/gossip | jq
curl http://localhost:8080/debug/cache | jq
```

**Security Note**:
These endpoints should be protected or disabled in production. Add configuration:

```go
type APIConfig struct {
    EnableDebug bool   `yaml:"enable_debug" env:"API_ENABLE_DEBUG"`
    DebugToken  string `yaml:"debug_token" env:"API_DEBUG_TOKEN"` // Optional auth
}

func (s *Server) registerDebugRoutes() {
    if !s.config.EnableDebug {
        return // Skip debug endpoints in production
    }

    // Optional: Add authentication middleware
    debugHandler := s.requireDebugAuth(http.HandlerFunc(s.handleDebugPeers))
    s.router.Handle("/debug/peers", debugHandler)
}
```

---

## Updated Deliverables

Phase 4 now includes:

- [x] Postgres backing store
- [x] **MySQL backing store** (NEW)
- [x] EC2, Docker, K8s discovery
- [x] **DNS-based discovery** (NEW)
- [x] HTTP API
- [x] **Debug endpoints** (NEW)
- [x] **pprof profiling** (NEW)
- [x] Prometheus metrics
- [x] Health checks
- [x] Graceful shutdown
- [x] Deployment manifests
- [x] Load testing
- [x] Documentation

## Updated Timeline

**Original**: 2-3 weeks (14-21 days)
**With Additions**: 2.5-3.5 weeks (17-24 days)

Additional time needed:
- MySQL backing store: +1 day
- DNS discovery: +1 day
- Debug/pprof endpoints: +1 day

**Total**: ~3 days additional development time

## Success Criteria Updates

Add to original success criteria:

1. **MySQL Support**: MySQL backing store works with version tracking
2. **DNS Discovery**: Can discover peers via DNS SRV and A records
3. **Debuggability**: pprof and debug endpoints provide operational insight
4. **Performance**: Profiling shows no hotspots under load

## Testing Requirements

Add these tests before implementing each addendum feature. Keep unit tests untagged and deterministic; reserve tagged integration tests for real MySQL or DNS behavior.

### MySQL Tests
```bash
# Integration test with MySQL
go test ./internal/backingstore/mysql/ -tags=integration
```

TDD slices:
- `internal/backingstore/mysql/mysql_test.go`: adapter builds queries, maps not-found errors, and preserves version semantics through a fake DB.
- `internal/backingstore/mysql/mysql_integration_test.go`: real MySQL round-trips values and increments versions transactionally.

### DNS Discovery Tests
```bash
# Test DNS discovery (requires local DNS or mock)
go test ./internal/discovery/ -run TestDNS
```

TDD slices:
- `internal/discovery/dns_test.go`: SRV records become peer addresses, self is filtered out, duplicate records collapse.
- `internal/discovery/dns_test.go`: resolver falls back to A records when SRV lookup is unavailable.
- `internal/discovery/dns_test.go`: TTL refresh keeps the last good peer set when a transient lookup fails.

### Debug Endpoint Tests
```bash
# Test debug endpoints
go test ./internal/api/ -run TestDebug
```

TDD slices:
- `internal/api/debug_test.go`: `/debug/peers`, `/debug/gossip`, and `/debug/cache` are disabled unless configured.
- `internal/api/debug_test.go`: enabled debug endpoints redact secrets and return stable JSON shapes.
- `internal/api/pprof_test.go`: pprof routes are registered only when profiling is explicitly enabled.

## Configuration Updates

Add to config.yaml:

```yaml
backing_store:
  type: mysql  # or postgres, redis
  address: localhost:3306
  database: gossipcache
  username: root
  password: secret

discovery:
  mode: dns  # or ec2, docker, kubernetes, static
  dns:
    service_name: gossipcache
    domain: default.svc.cluster.local
    port: 7946
    interval: 30s

api:
  enable_debug: false  # Set to true only in non-production
  debug_token: secret_token  # Optional authentication
```

## Documentation Updates

Update docs/DEPLOYMENT.md with:

1. **MySQL Deployment Section**:
   - Schema setup
   - Connection pooling
   - Performance tuning

2. **DNS Discovery Section**:
   - Kubernetes headless services
   - DNS SRV records
   - Fallback to A records

3. **Debugging Guide**:
   - Using pprof for performance analysis
   - Debug endpoints for troubleshooting
   - Common issues and solutions

---

## Next Steps

After completing Phase 4 + Addendum:

1. Run full test suite
2. Perform load testing with all backing stores
3. Validate DNS discovery in K8s
4. Profile with pprof under load
5. Update all documentation
6. Create release candidate

---

## Optional: Phase 4.5 - Security (Future)

If security is required before launch, use [Phase 4.5 Security](PHASE_4_5_SECURITY.md):

### Objectives
- [ ] TLS for gossip protocol
- [ ] mTLS for node authentication
- [ ] API authentication (JWT or API keys)
- [ ] Rate limiting
- [ ] Audit logging

**Duration**: 1-2 weeks

**Priority**: Low (can be v2 feature)

Most deployments will use trusted networks initially. Security can be added post-launch based on user requirements.

---

## Summary

This addendum adds **3 high-priority features** to Phase 4:

1. ✅ MySQL backing store (broad user base)
2. ✅ DNS discovery (K8s standard pattern)
3. ✅ Debug/pprof endpoints (operational necessity)

These additions increase implementation time by ~3 days but significantly improve production readiness and operational capabilities.

**Total Phase 4 Duration**: 17-24 days (2.5-3.5 weeks)

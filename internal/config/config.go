// Package config loads and validates process configuration for GossipCache.
//
// v1 product shape is hybrid: in-process L1 + native L2 hub
// (docs/SEMANTICS.md). This package holds settings for local L1 behavior and
// hub connectivity placeholders used by later phases. Independent full-value
// gossip and Redis-as-source-of-truth are not configured here.
package config

import "time"

// Config is the complete process configuration.
type Config struct {
	// NodeID identifies this process in logs and (later) stream confirms.
	// Empty NodeID is filled from the hostname during Validate.
	NodeID string `yaml:"node_id" env:"NODE_ID"`

	Cache   CacheConfig   `yaml:"cache"`
	L2      L2Config      `yaml:"l2"`
	Logging LoggingConfig `yaml:"logging"`
	Metrics MetricsConfig `yaml:"metrics"`
}

// CacheConfig controls the local in-memory L1 engine.
type CacheConfig struct {
	MaxSize        int64         `yaml:"max_size" env:"CACHE_MAX_SIZE"`
	DefaultTTL     time.Duration `yaml:"default_ttl" env:"CACHE_DEFAULT_TTL"`
	EvictionPolicy string        `yaml:"eviction_policy" env:"CACHE_EVICTION_POLICY"`
	MaxKeySize     int           `yaml:"max_key_size"`
	MaxValueSize   int           `yaml:"max_value_size"`
	// StalePolicy is reserved for the L1 state machine (P1). Values: never,
	// stale_if_error, serve_stale_while_revalidate. Default never.
	StalePolicy string `yaml:"stale_policy" env:"CACHE_STALE_POLICY"`
}

// L2Config holds hub connectivity and write visibility knobs (SEMANTICS §8–10).
// Addresses may be empty until an L2 hub is wired (P3); local L1-only use is fine.
type L2Config struct {
	// Addresses are hub RPC endpoints (host:port). Multiple entries are for HA later.
	Addresses []string `yaml:"addresses"`

	// RPCPort is the default port used when an address omits a port (placeholder 7400).
	RPCPort int `yaml:"rpc_port" env:"L2_RPC_PORT"`
	// StreamPort is the invalidation/control stream port (placeholder 7401).
	StreamPort int `yaml:"stream_port" env:"L2_STREAM_PORT"`

	// StreamFreshnessTimeout is max age of last origin StreamCheckpoint before
	// readiness fails with STREAM_FRESHNESS_UNKNOWN (SEMANTICS §9–10).
	StreamFreshnessTimeout time.Duration `yaml:"stream_freshness_timeout" env:"L2_STREAM_FRESHNESS_TIMEOUT"`

	// DefaultWriteW is peer confirms before OK; 0 = async peers (default).
	DefaultWriteW int `yaml:"default_write_w" env:"L2_DEFAULT_WRITE_W"`

	// MgmtListen is the HTTP bind for /livez /startupz /readyz (and later debug).
	MgmtListen string `yaml:"mgmt_listen" env:"L2_MGMT_LISTEN"`
}

// LoggingConfig controls slog output.
type LoggingConfig struct {
	Level  string `yaml:"level" env:"LOG_LEVEL"`
	Format string `yaml:"format" env:"LOG_FORMAT"` // json or text
}

// MetricsConfig controls the Prometheus scrape endpoint.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" env:"METRICS_ENABLED"`
	Listen  string `yaml:"listen" env:"METRICS_LISTEN"` // e.g. ":9090"; Port alone is legacy
	Port    int    `yaml:"port" env:"METRICS_PORT"`     // used when Listen is empty
}

// Default returns configuration suitable for local L1 development.
// Hub fields use the documented port placeholders from DEPLOYMENT.md.
func Default() *Config {
	return &Config{
		NodeID: "",
		Cache: CacheConfig{
			MaxSize:        1 << 30, // 1 GiB soft cap
			DefaultTTL:     5 * time.Minute,
			EvictionPolicy: "lru",
			MaxKeySize:     1024,
			MaxValueSize:   10 << 20, // 10 MiB
			StalePolicy:    "never",
		},
		L2: L2Config{
			Addresses:              nil,
			RPCPort:                7400,
			StreamPort:             7401,
			StreamFreshnessTimeout: 3 * time.Second,
			DefaultWriteW:          0,
			MgmtListen:             "127.0.0.1:8081",
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

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaultsWhenNoPathProvided(t *testing.T) {
	t.Setenv("NODE_ID", "")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.NodeID == "" {
		t.Fatal("NodeID is empty, want hostname populated during validation")
	}
	if cfg.L2.DefaultWriteW != 0 {
		t.Fatalf("DefaultWriteW = %d, want 0", cfg.L2.DefaultWriteW)
	}
}

func TestLoadReadsYAMLFile(t *testing.T) {
	t.Setenv("NODE_ID", "")

	path := writeConfigFile(t, `
node_id: file-node
cache:
  max_size: 2048
  default_ttl: 30s
  eviction_policy: lru
  max_key_size: 128
  max_value_size: 4096
  stale_policy: never
l2:
  addresses:
    - 127.0.0.1:7400
  rpc_port: 7400
  stream_port: 7401
  stream_freshness_timeout: 5s
  default_write_w: 0
  mgmt_listen: 127.0.0.1:8081
logging:
  level: debug
  format: text
metrics:
  enabled: false
  port: 9100
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.NodeID != "file-node" {
		t.Fatalf("NodeID = %q, want %q", cfg.NodeID, "file-node")
	}
	if cfg.Cache.MaxSize != 2048 {
		t.Fatalf("Cache.MaxSize = %d, want %d", cfg.Cache.MaxSize, 2048)
	}
	if cfg.Cache.DefaultTTL != 30*time.Second {
		t.Fatalf("Cache.DefaultTTL = %s, want %s", cfg.Cache.DefaultTTL, 30*time.Second)
	}
	if len(cfg.L2.Addresses) != 1 || cfg.L2.Addresses[0] != "127.0.0.1:7400" {
		t.Fatalf("L2.Addresses = %v, want [127.0.0.1:7400]", cfg.L2.Addresses)
	}
	if cfg.L2.StreamFreshnessTimeout != 5*time.Second {
		t.Fatalf("StreamFreshnessTimeout = %s, want 5s", cfg.L2.StreamFreshnessTimeout)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled = true, want false")
	}
	if cfg.Metrics.Port != 9100 {
		t.Fatalf("Metrics.Port = %d, want %d", cfg.Metrics.Port, 9100)
	}
}

func TestLoadEnvironmentOverridesFile(t *testing.T) {
	t.Setenv("NODE_ID", "env-node")
	t.Setenv("L2_DEFAULT_WRITE_W", "2")

	path := writeConfigFile(t, `
node_id: file-node
cache:
  max_size: 1024
l2:
  default_write_w: 0
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.NodeID != "env-node" {
		t.Fatalf("NodeID = %q, want %q", cfg.NodeID, "env-node")
	}
	if cfg.L2.DefaultWriteW != 2 {
		t.Fatalf("DefaultWriteW = %d, want 2", cfg.L2.DefaultWriteW)
	}
}

func TestLoadWrapsFileErrors(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yml"))
	if err == nil {
		t.Fatal("Load returned nil error, want file error")
	}
	if !strings.Contains(err.Error(), "load config file") {
		t.Fatalf("error = %q, want load config file context", err)
	}
}

func TestLoadWrapsValidationErrors(t *testing.T) {
	t.Setenv("NODE_ID", "")

	path := writeConfigFile(t, `
cache:
  max_size: 0
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error, want validation error")
	}
	if !strings.Contains(err.Error(), "validate config") {
		t.Fatalf("error = %q, want validate config context", err)
	}
}

func TestLoadEnvOverridesAllTaggedFields(t *testing.T) {
	t.Setenv("NODE_ID", "env-node")
	t.Setenv("CACHE_MAX_SIZE", "4096")
	t.Setenv("CACHE_DEFAULT_TTL", "45s")
	t.Setenv("CACHE_EVICTION_POLICY", "lru")
	t.Setenv("CACHE_STALE_POLICY", "stale_if_error")
	t.Setenv("L2_ADDRESSES", "hub1:7400, hub2:7400")
	t.Setenv("L2_RPC_PORT", "7400")
	t.Setenv("L2_STREAM_PORT", "7401")
	t.Setenv("L2_STREAM_FRESHNESS_TIMEOUT", "4s")
	t.Setenv("L2_DEFAULT_WRITE_W", "0")
	t.Setenv("L2_MGMT_LISTEN", "127.0.0.1:8082")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("METRICS_ENABLED", "false")
	t.Setenv("METRICS_PORT", "9999")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.NodeID != "env-node" {
		t.Errorf("NodeID = %v, want env-node", cfg.NodeID)
	}
	if cfg.Cache.MaxSize != 4096 {
		t.Errorf("Cache.MaxSize = %v, want 4096", cfg.Cache.MaxSize)
	}
	if cfg.Cache.DefaultTTL != 45*time.Second {
		t.Errorf("Cache.DefaultTTL = %v, want 45s", cfg.Cache.DefaultTTL)
	}
	if cfg.Cache.StalePolicy != "stale_if_error" {
		t.Errorf("StalePolicy = %v, want stale_if_error", cfg.Cache.StalePolicy)
	}
	if len(cfg.L2.Addresses) != 2 {
		t.Fatalf("L2.Addresses = %v, want 2 entries", cfg.L2.Addresses)
	}
	if cfg.L2.MgmtListen != "127.0.0.1:8082" {
		t.Errorf("MgmtListen = %v, want 127.0.0.1:8082", cfg.L2.MgmtListen)
	}
	if cfg.L2.StreamFreshnessTimeout != 4*time.Second {
		t.Errorf("StreamFreshnessTimeout = %v, want 4s", cfg.L2.StreamFreshnessTimeout)
	}
	if cfg.Metrics.Port != 9999 {
		t.Errorf("Metrics.Port = %v, want 9999", cfg.Metrics.Port)
	}
}

func TestLoadEnvWrapsParseErrors(t *testing.T) {
	tests := []struct {
		envVar string
		value  string
		name   string
	}{
		{envVar: "CACHE_MAX_SIZE", value: "not-an-int", name: "CACHE_MAX_SIZE"},
		{envVar: "CACHE_DEFAULT_TTL", value: "not-a-duration", name: "CACHE_DEFAULT_TTL"},
		{envVar: "L2_RPC_PORT", value: "abc", name: "L2_RPC_PORT"},
		{envVar: "L2_DEFAULT_WRITE_W", value: "x", name: "L2_DEFAULT_WRITE_W"},
		{envVar: "METRICS_ENABLED", value: "maybe", name: "METRICS_ENABLED"},
		{envVar: "METRICS_PORT", value: "abc", name: "METRICS_PORT"},
	}
	for _, tt := range tests {
		t.Run(tt.envVar, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.value)
			_, err := Load("")
			if err == nil {
				t.Fatalf("Load returned nil error, want parse error for %s", tt.envVar)
			}
			if !strings.Contains(err.Error(), "load config from env") {
				t.Errorf("error = %q, want load config from env context", err)
			}
			if !strings.Contains(err.Error(), tt.name) {
				t.Errorf("error = %q, want substring %q", err, tt.name)
			}
		})
	}
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	return path
}

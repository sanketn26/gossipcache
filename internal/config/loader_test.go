package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaultsWhenNoPathProvided(t *testing.T) {
	t.Setenv("MODE", "")
	t.Setenv("NODE_ID", "")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Mode != ModeBacked {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeBacked)
	}
	if cfg.NodeID == "" {
		t.Fatal("NodeID is empty, want hostname populated during validation")
	}
}

func TestLoadReadsYAMLFile(t *testing.T) {
	t.Setenv("MODE", "")
	t.Setenv("NODE_ID", "")

	path := writeConfigFile(t, `
mode: independent
node_id: file-node
address: 127.0.0.1:9000
cache:
  max_size: 2048
  default_ttl: 30s
  eviction_policy: fifo
  max_key_size: 128
  max_value_size: 4096
gossip:
  interval: 2s
  fanout: 5
  anti_entropy_interval: 1m
network:
  tcp_port: 9001
  udp_port: 9002
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

	if cfg.Mode != ModeIndependent {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeIndependent)
	}
	if cfg.NodeID != "file-node" {
		t.Fatalf("NodeID = %q, want %q", cfg.NodeID, "file-node")
	}
	if cfg.Address != "127.0.0.1:9000" {
		t.Fatalf("Address = %q, want %q", cfg.Address, "127.0.0.1:9000")
	}
	if cfg.Cache.MaxSize != 2048 {
		t.Fatalf("Cache.MaxSize = %d, want %d", cfg.Cache.MaxSize, 2048)
	}
	if cfg.Cache.DefaultTTL != 30*time.Second {
		t.Fatalf("Cache.DefaultTTL = %s, want %s", cfg.Cache.DefaultTTL, 30*time.Second)
	}
	if cfg.Cache.EvictionPolicy != "fifo" {
		t.Fatalf("Cache.EvictionPolicy = %q, want %q", cfg.Cache.EvictionPolicy, "fifo")
	}
	if cfg.Gossip.Interval != 2*time.Second {
		t.Fatalf("Gossip.Interval = %s, want %s", cfg.Gossip.Interval, 2*time.Second)
	}
	if cfg.Gossip.Fanout != 5 {
		t.Fatalf("Gossip.Fanout = %d, want %d", cfg.Gossip.Fanout, 5)
	}
	if cfg.Network.TCPPort != 9001 {
		t.Fatalf("Network.TCPPort = %d, want %d", cfg.Network.TCPPort, 9001)
	}
	if cfg.Network.UDPPort != 9002 {
		t.Fatalf("Network.UDPPort = %d, want %d", cfg.Network.UDPPort, 9002)
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

func TestLoadEnvironmentOverridesFileForImplementedVariables(t *testing.T) {
	t.Setenv("MODE", string(ModeIndependent))
	t.Setenv("NODE_ID", "env-node")

	path := writeConfigFile(t, `
mode: backed
node_id: file-node
cache:
  max_size: 1024
gossip:
  fanout: 3
network:
  tcp_port: 7946
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Mode != ModeIndependent {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeIndependent)
	}
	if cfg.NodeID != "env-node" {
		t.Fatalf("NodeID = %q, want %q", cfg.NodeID, "env-node")
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
	t.Setenv("MODE", "")
	t.Setenv("NODE_ID", "")

	path := writeConfigFile(t, `
mode: invalid
cache:
  max_size: 1024
gossip:
  fanout: 3
network:
  tcp_port: 7946
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error, want validation error")
	}
	if !strings.Contains(err.Error(), "validate config") {
		t.Fatalf("error = %q, want validate config context", err)
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

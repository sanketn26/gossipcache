package config

import (
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.NodeID != "" {
		t.Fatalf("NodeID = %q, want empty", cfg.NodeID)
	}
	if cfg.Cache.MaxSize != 1<<30 {
		t.Fatalf("Cache.MaxSize = %d, want %d", cfg.Cache.MaxSize, int64(1<<30))
	}
	if cfg.Cache.DefaultTTL != 5*time.Minute {
		t.Fatalf("Cache.DefaultTTL = %s, want %s", cfg.Cache.DefaultTTL, 5*time.Minute)
	}
	if cfg.Cache.EvictionPolicy != "lru" {
		t.Fatalf("Cache.EvictionPolicy = %q, want %q", cfg.Cache.EvictionPolicy, "lru")
	}
	if cfg.Cache.MaxKeySize != 1024 {
		t.Fatalf("Cache.MaxKeySize = %d, want %d", cfg.Cache.MaxKeySize, 1024)
	}
	if cfg.Cache.MaxValueSize != 10<<20 {
		t.Fatalf("Cache.MaxValueSize = %d, want %d", cfg.Cache.MaxValueSize, 10<<20)
	}
	if cfg.Cache.StalePolicy != "never" {
		t.Fatalf("Cache.StalePolicy = %q, want never", cfg.Cache.StalePolicy)
	}
	if cfg.L2.RPCPort != 7400 {
		t.Fatalf("L2.RPCPort = %d, want 7400", cfg.L2.RPCPort)
	}
	if cfg.L2.StreamPort != 7401 {
		t.Fatalf("L2.StreamPort = %d, want 7401", cfg.L2.StreamPort)
	}
	if cfg.L2.StreamFreshnessTimeout != 3*time.Second {
		t.Fatalf("L2.StreamFreshnessTimeout = %s, want 3s", cfg.L2.StreamFreshnessTimeout)
	}
	if cfg.L2.DefaultWriteW != 0 {
		t.Fatalf("L2.DefaultWriteW = %d, want 0", cfg.L2.DefaultWriteW)
	}
	if cfg.L2.MgmtListen != "127.0.0.1:8081" {
		t.Fatalf("L2.MgmtListen = %q, want 127.0.0.1:8081", cfg.L2.MgmtListen)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("Logging.Level = %q, want %q", cfg.Logging.Level, "info")
	}
	if cfg.Logging.Format != "json" {
		t.Fatalf("Logging.Format = %q, want %q", cfg.Logging.Format, "json")
	}
	if !cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled = false, want true")
	}
	if cfg.Metrics.Port != 9090 {
		t.Fatalf("Metrics.Port = %d, want %d", cfg.Metrics.Port, 9090)
	}
}

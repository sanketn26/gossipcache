package config

import (
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Mode != ModeBacked {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeBacked)
	}
	if cfg.NodeID != "" {
		t.Fatalf("NodeID = %q, want empty", cfg.NodeID)
	}
	if cfg.Address != "0.0.0.0:7946" {
		t.Fatalf("Address = %q, want %q", cfg.Address, "0.0.0.0:7946")
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
	if cfg.Gossip.Interval != time.Second {
		t.Fatalf("Gossip.Interval = %s, want %s", cfg.Gossip.Interval, time.Second)
	}
	if cfg.Gossip.Fanout != 3 {
		t.Fatalf("Gossip.Fanout = %d, want %d", cfg.Gossip.Fanout, 3)
	}
	if cfg.Gossip.AntiEntropyInterval != 5*time.Minute {
		t.Fatalf("Gossip.AntiEntropyInterval = %s, want %s", cfg.Gossip.AntiEntropyInterval, 5*time.Minute)
	}
	if cfg.Network.TCPPort != 7946 {
		t.Fatalf("Network.TCPPort = %d, want %d", cfg.Network.TCPPort, 7946)
	}
	if cfg.Network.UDPPort != 7946 {
		t.Fatalf("Network.UDPPort = %d, want %d", cfg.Network.UDPPort, 7946)
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

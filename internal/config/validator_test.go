package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAcceptsValidConfigAndPopulatesNodeID(t *testing.T) {
	cfg := Default()
	cfg.NodeID = ""

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if cfg.NodeID == "" {
		t.Fatal("NodeID is empty, want hostname populated")
	}
	if cfg.Metrics.Listen != ":9090" {
		t.Fatalf("Metrics.Listen = %q, want :9090 after normalize", cfg.Metrics.Listen)
	}
}

func TestValidateAllowsExplicitNodeID(t *testing.T) {
	cfg := Default()
	cfg.NodeID = "explicit-node"

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if cfg.NodeID != "explicit-node" {
		t.Fatalf("NodeID = %q, want %q", cfg.NodeID, "explicit-node")
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "non-positive cache size",
			mutate: func(cfg *Config) {
				cfg.Cache.MaxSize = 0
			},
			wantErr: "cache.max_size must be positive",
		},
		{
			name: "negative ttl",
			mutate: func(cfg *Config) {
				cfg.Cache.DefaultTTL = -time.Second
			},
			wantErr: "cache.default_ttl cannot be negative",
		},
		{
			name: "bad stale policy",
			mutate: func(cfg *Config) {
				cfg.Cache.StalePolicy = "always"
			},
			wantErr: "invalid cache.stale_policy",
		},
		{
			name: "unsupported eviction",
			mutate: func(cfg *Config) {
				cfg.Cache.EvictionPolicy = "fifo"
			},
			wantErr: "cache.eviction_policy",
		},
		{
			name: "rpc port too low",
			mutate: func(cfg *Config) {
				cfg.L2.RPCPort = 0
			},
			wantErr: "invalid l2.rpc_port",
		},
		{
			name: "stream port too high",
			mutate: func(cfg *Config) {
				cfg.L2.StreamPort = 65536
			},
			wantErr: "invalid l2.stream_port",
		},
		{
			name: "negative write W",
			mutate: func(cfg *Config) {
				cfg.L2.DefaultWriteW = -1
			},
			wantErr: "l2.default_write_w cannot be negative",
		},
		{
			name: "bad mgmt listen",
			mutate: func(cfg *Config) {
				cfg.L2.MgmtListen = "not-a-listen-addr"
			},
			wantErr: "invalid l2.mgmt_listen",
		},
		{
			name: "empty l2 address entry",
			mutate: func(cfg *Config) {
				cfg.L2.Addresses = []string{"hub:7400", "  "}
			},
			wantErr: "l2.addresses[1] is empty",
		},
		{
			name: "negative max_key_size",
			mutate: func(cfg *Config) {
				cfg.Cache.MaxKeySize = -1
			},
			wantErr: "cache.max_key_size cannot be negative",
		},
		{
			name: "negative max_value_size",
			mutate: func(cfg *Config) {
				cfg.Cache.MaxValueSize = -1
			},
			wantErr: "cache.max_value_size cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.NodeID = "node-1"
			tt.mutate(cfg)

			err := Validate(cfg)
			if err == nil {
				t.Fatal("Validate returned nil error, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

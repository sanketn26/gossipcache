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
			name: "invalid mode",
			mutate: func(cfg *Config) {
				cfg.Mode = OperatingMode("bad")
			},
			wantErr: "invalid mode",
		},
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
			name: "non-positive gossip fanout",
			mutate: func(cfg *Config) {
				cfg.Gossip.Fanout = 0
			},
			wantErr: "gossip.fanout must be positive",
		},
		{
			name: "tcp port too low",
			mutate: func(cfg *Config) {
				cfg.Network.TCPPort = 0
			},
			wantErr: "invalid tcp_port",
		},
		{
			name: "tcp port too high",
			mutate: func(cfg *Config) {
				cfg.Network.TCPPort = 65536
			},
			wantErr: "invalid tcp_port",
		},
		{
			name: "udp port too low",
			mutate: func(cfg *Config) {
				cfg.Network.UDPPort = 0
			},
			wantErr: "invalid udp_port",
		},
		{
			name: "udp port too high",
			mutate: func(cfg *Config) {
				cfg.Network.UDPPort = 65536
			},
			wantErr: "invalid udp_port",
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

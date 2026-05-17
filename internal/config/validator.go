package config

import (
	"errors"
	"fmt"
	"os"
)

// Validate validates the configuration. As a side effect, it populates an
// empty NodeID from the hostname.
func Validate(cfg *Config) error {
	if cfg.Mode != ModeBacked && cfg.Mode != ModeIndependent {
		return fmt.Errorf("invalid mode: %s (must be 'backed' or 'independent')", cfg.Mode)
	}

	if cfg.NodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("get hostname for node_id: %w", err)
		}
		cfg.NodeID = hostname
	}

	if cfg.Cache.MaxSize <= 0 {
		return errors.New("cache.max_size must be positive")
	}

	if cfg.Cache.DefaultTTL < 0 {
		return errors.New("cache.default_ttl cannot be negative")
	}

	if cfg.Cache.MaxKeySize < 0 {
		return errors.New("cache.max_key_size cannot be negative")
	}
	if cfg.Cache.MaxValueSize < 0 {
		return errors.New("cache.max_value_size cannot be negative")
	}

	if cfg.Gossip.Fanout <= 0 {
		return errors.New("gossip.fanout must be positive")
	}

	if cfg.Network.TCPPort <= 0 || cfg.Network.TCPPort > 65535 {
		return fmt.Errorf("invalid tcp_port: %d", cfg.Network.TCPPort)
	}
	if cfg.Network.UDPPort <= 0 || cfg.Network.UDPPort > 65535 {
		return fmt.Errorf("invalid udp_port: %d", cfg.Network.UDPPort)
	}

	return nil
}

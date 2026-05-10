// internal/config/validator.go
package config

import (
	"errors"
	"fmt"
	"os"
)

// Validate validates the configuration
func Validate(cfg *Config) error {
	// Validate mode
	if cfg.Mode != ModeBacked && cfg.Mode != ModeIndependent {
		return fmt.Errorf("invalid mode: %s (must be 'backed' or 'independent')", cfg.Mode)
	}

	// Set NodeID if empty
	if cfg.NodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("get hostname for node_id: %w", err)
		}
		cfg.NodeID = hostname
	}

	// Validate cache settings
	if cfg.Cache.MaxSize <= 0 {
		return errors.New("cache.max_size must be positive")
	}

	if cfg.Cache.DefaultTTL < 0 {
		return errors.New("cache.default_ttl cannot be negative")
	}

	// Validate gossip settings
	if cfg.Gossip.Fanout <= 0 {
		return errors.New("gossip.fanout must be positive")
	}

	// Validate network ports
	if cfg.Network.TCPPort <= 0 || cfg.Network.TCPPort > 65535 {
		return fmt.Errorf("invalid tcp_port: %d", cfg.Network.TCPPort)
	}

	return nil
}

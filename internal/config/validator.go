package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

// Validate validates the configuration. As a side effect, it populates an
// empty NodeID from the hostname and normalizes Metrics.Listen from Port.
func Validate(cfg *Config) error {
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
	if err := validateStalePolicy(cfg.Cache.StalePolicy); err != nil {
		return err
	}
	if cfg.Cache.EvictionPolicy != "" && !strings.EqualFold(cfg.Cache.EvictionPolicy, "lru") {
		return fmt.Errorf("cache.eviction_policy %q not supported (only lru)", cfg.Cache.EvictionPolicy)
	}

	if err := validatePort("l2.rpc_port", cfg.L2.RPCPort); err != nil {
		return err
	}
	if err := validatePort("l2.stream_port", cfg.L2.StreamPort); err != nil {
		return err
	}
	if cfg.L2.StreamFreshnessTimeout < 0 {
		return errors.New("l2.stream_freshness_timeout cannot be negative")
	}
	if cfg.L2.DefaultWriteW < 0 {
		return errors.New("l2.default_write_w cannot be negative")
	}
	if cfg.L2.MgmtListen != "" {
		if _, _, err := net.SplitHostPort(cfg.L2.MgmtListen); err != nil {
			return fmt.Errorf("invalid l2.mgmt_listen %q: %w", cfg.L2.MgmtListen, err)
		}
	}
	for i, addr := range cfg.L2.Addresses {
		if strings.TrimSpace(addr) == "" {
			return fmt.Errorf("l2.addresses[%d] is empty", i)
		}
	}

	if cfg.Metrics.Listen == "" {
		// Port 0 means ephemeral bind (tests); still a valid listen form.
		if cfg.Metrics.Port < 0 || cfg.Metrics.Port > 65535 {
			return fmt.Errorf("invalid metrics.port: %d", cfg.Metrics.Port)
		}
		cfg.Metrics.Listen = fmt.Sprintf(":%d", cfg.Metrics.Port)
	}
	if cfg.Metrics.Enabled {
		if _, _, err := net.SplitHostPort(cfg.Metrics.Listen); err != nil {
			return fmt.Errorf("invalid metrics.listen %q: %w", cfg.Metrics.Listen, err)
		}
	}

	return nil
}

func validateStalePolicy(p string) error {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", "never", "stale_if_error", "serve_stale_while_revalidate":
		return nil
	default:
		return fmt.Errorf("invalid cache.stale_policy %q (never|stale_if_error|serve_stale_while_revalidate)", p)
	}
}

func validatePort(name string, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid %s: %d", name, port)
	}
	return nil
}

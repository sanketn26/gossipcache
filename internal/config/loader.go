package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Load loads configuration from file (if path is non-empty), then overlays
// environment variables, then validates.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		if err := loadFromFile(cfg, path); err != nil {
			return nil, fmt.Errorf("load config file: %w", err)
		}
	}

	if err := loadFromEnv(cfg); err != nil {
		return nil, fmt.Errorf("load config from env: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func loadFromFile(cfg *Config, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	return decoder.Decode(cfg)
}

func loadFromEnv(cfg *Config) error {
	if v := os.Getenv("MODE"); v != "" {
		cfg.Mode = OperatingMode(v)
	}
	if v := os.Getenv("NODE_ID"); v != "" {
		cfg.NodeID = v
	}
	if v := os.Getenv("ADDRESS"); v != "" {
		cfg.Address = v
	}

	if v := os.Getenv("CACHE_MAX_SIZE"); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("parse CACHE_MAX_SIZE=%q: %w", v, err)
		}
		cfg.Cache.MaxSize = parsed
	}
	if v := os.Getenv("CACHE_DEFAULT_TTL"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("parse CACHE_DEFAULT_TTL=%q: %w", v, err)
		}
		cfg.Cache.DefaultTTL = parsed
	}
	if v := os.Getenv("CACHE_EVICTION_POLICY"); v != "" {
		cfg.Cache.EvictionPolicy = v
	}

	if v := os.Getenv("TCP_PORT"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse TCP_PORT=%q: %w", v, err)
		}
		cfg.Network.TCPPort = parsed
	}
	if v := os.Getenv("UDP_PORT"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse UDP_PORT=%q: %w", v, err)
		}
		cfg.Network.UDPPort = parsed
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}

	if v := os.Getenv("METRICS_ENABLED"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse METRICS_ENABLED=%q: %w", v, err)
		}
		cfg.Metrics.Enabled = parsed
	}
	if v := os.Getenv("METRICS_PORT"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse METRICS_PORT=%q: %w", v, err)
		}
		cfg.Metrics.Port = parsed
	}

	return nil
}

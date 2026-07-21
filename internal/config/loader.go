package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	if v := os.Getenv("NODE_ID"); v != "" {
		cfg.NodeID = v
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
	if v := os.Getenv("CACHE_STALE_POLICY"); v != "" {
		cfg.Cache.StalePolicy = v
	}

	if v := os.Getenv("L2_ADDRESSES"); v != "" {
		cfg.L2.Addresses = splitCSV(v)
	}
	if v := os.Getenv("L2_RPC_PORT"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse L2_RPC_PORT=%q: %w", v, err)
		}
		cfg.L2.RPCPort = parsed
	}
	if v := os.Getenv("L2_STREAM_PORT"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse L2_STREAM_PORT=%q: %w", v, err)
		}
		cfg.L2.StreamPort = parsed
	}
	if v := os.Getenv("L2_STREAM_FRESHNESS_TIMEOUT"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("parse L2_STREAM_FRESHNESS_TIMEOUT=%q: %w", v, err)
		}
		cfg.L2.StreamFreshnessTimeout = parsed
	}
	if v := os.Getenv("L2_DEFAULT_WRITE_W"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse L2_DEFAULT_WRITE_W=%q: %w", v, err)
		}
		cfg.L2.DefaultWriteW = parsed
	}
	if v := os.Getenv("L2_MGMT_LISTEN"); v != "" {
		cfg.L2.MgmtListen = v
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
	if v := os.Getenv("METRICS_LISTEN"); v != "" {
		cfg.Metrics.Listen = v
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

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

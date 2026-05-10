// internal/config/loader.go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load loads configuration from file and environment
// DRY: Single function handles all config loading
func Load(path string) (*Config, error) {
	cfg := Default()

	// Load from file if provided
	if path != "" {
		if err := loadFromFile(cfg, path); err != nil {
			return nil, fmt.Errorf("load config file: %w", err)
		}
	}

	// Override with environment variables
	if err := loadFromEnv(cfg); err != nil {
		return nil, fmt.Errorf("load config from env: %w", err)
	}

	// Validate
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
	// Use reflection or manual parsing
	// For simplicity, manual parsing shown

	if mode := os.Getenv("MODE"); mode != "" {
		cfg.Mode = OperatingMode(mode)
	}

	if nodeID := os.Getenv("NODE_ID"); nodeID != "" {
		cfg.NodeID = nodeID
	}

	// ... more environment variable parsing

	return nil
}

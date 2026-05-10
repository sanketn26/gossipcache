package config

import "time"

// Config represents the complete configuration
type Config struct {
	Mode    OperatingMode `yaml:"mode" env:"MODE"`
	NodeID  string        `yaml:"node_id" env:"NODE_ID"`
	Address string        `yaml:"address" env:"ADDRESS"`

	Cache   CacheConfig   `yaml:"cache"`
	Gossip  GossipConfig  `yaml:"gossip"`
	Network NetworkConfig `yaml:"network"`
	Logging LoggingConfig `yaml:"logging"`
	Metrics MetricsConfig `yaml:"metrics"`
}

type OperatingMode string

const (
	ModeBacked      OperatingMode = "backed"
	ModeIndependent OperatingMode = "independent"
)

type CacheConfig struct {
	MaxSize        int64         `yaml:"max_size" env:"CACHE_MAX_SIZE"`
	DefaultTTL     time.Duration `yaml:"default_ttl" env:"CACHE_DEFAULT_TTL"`
	EvictionPolicy string        `yaml:"eviction_policy" env:"CACHE_EVICTION_POLICY"`
	MaxKeySize     int           `yaml:"max_key_size"`
	MaxValueSize   int           `yaml:"max_value_size"`
}

type GossipConfig struct {
	Interval            time.Duration `yaml:"interval"`
	Fanout              int           `yaml:"fanout"`
	AntiEntropyInterval time.Duration `yaml:"anti_entropy_interval"`
}

type NetworkConfig struct {
	TCPPort int `yaml:"tcp_port" env:"TCP_PORT"`
	UDPPort int `yaml:"udp_port" env:"UDP_PORT"`
}

type LoggingConfig struct {
	Level  string `yaml:"level" env:"LOG_LEVEL"`
	Format string `yaml:"format" env:"LOG_FORMAT"` // json or text
}

type MetricsConfig struct {
	Enabled bool `yaml:"enabled" env:"METRICS_ENABLED"`
	Port    int  `yaml:"port" env:"METRICS_PORT"`
}

// Default returns a configuration with sensible defaults
func Default() *Config {
	return &Config{
		Mode:    ModeBacked,
		NodeID:  "", // Will be set to hostname
		Address: "0.0.0.0:7946",
		Cache: CacheConfig{
			MaxSize:        1 << 30, // 1GB
			DefaultTTL:     5 * time.Minute,
			EvictionPolicy: "lru",
			MaxKeySize:     1024,     // 1KB
			MaxValueSize:   10 << 20, // 10MB
		},
		Gossip: GossipConfig{
			Interval:            1 * time.Second,
			Fanout:              3,
			AntiEntropyInterval: 5 * time.Minute,
		},
		Network: NetworkConfig{
			TCPPort: 7946,
			UDPPort: 7946,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    9090,
		},
	}
}

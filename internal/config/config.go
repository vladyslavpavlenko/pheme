// Package config loads node settings from a YAML file and environment variables (envconfig prefix "pheme").
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"
)

// Config aggregates network, gossip, failure detection, bloom, health check, and API settings for one process.
type Config struct {
	BindAddr  string   `yaml:"bind_addr"  envconfig:"BIND_ADDR"`
	JoinAddrs []string `yaml:"join_addrs" envconfig:"JOIN_ADDRS"`
	Zone      string   `yaml:"zone"       envconfig:"ZONE"`
	NodeID    string   `yaml:"node_id"    envconfig:"NODE_ID"`

	GossipInterval time.Duration `yaml:"gossip_interval" envconfig:"GOSSIP_INTERVAL"`
	LocalWeight    float64       `yaml:"local_weight"    envconfig:"LOCAL_WEIGHT"`
	RemoteWeight   float64       `yaml:"remote_weight"   envconfig:"REMOTE_WEIGHT"`
	IndirectChecks int           `yaml:"indirect_checks" envconfig:"INDIRECT_CHECKS"`

	SuspicionMultiplier float64 `yaml:"suspicion_multiplier" envconfig:"SUSPICION_MULTIPLIER"`
	RTTWindowSize       int     `yaml:"rtt_window_size"      envconfig:"RTT_WINDOW_SIZE"`

	BloomExpectedItems    uint          `yaml:"bloom_expected_items"    envconfig:"BLOOM_EXPECTED_ITEMS"`
	BloomFPRate           float64       `yaml:"bloom_fp_rate"           envconfig:"BLOOM_FP_RATE"`
	BloomRotationInterval time.Duration `yaml:"bloom_rotation_interval" envconfig:"BLOOM_ROTATION_INTERVAL"`

	HealthcheckURL      string        `yaml:"healthcheck_url"      envconfig:"HEALTHCHECK_URL"`
	HealthcheckInterval time.Duration `yaml:"healthcheck_interval" envconfig:"HEALTHCHECK_INTERVAL"`

	APIAddr string `yaml:"api_addr" envconfig:"API_ADDR"`
}

// Default supplies non-zero field values used as the base before YAML and env overlays.
func Default() *Config {
	return &Config{
		BindAddr:  "0.0.0.0:7946",
		JoinAddrs: nil,
		Zone:      "default",
		NodeID:    "",

		GossipInterval: 200 * time.Millisecond,
		LocalWeight:    0.7,
		RemoteWeight:   0.3,
		IndirectChecks: 3,

		SuspicionMultiplier: 4,
		RTTWindowSize:       50,

		BloomExpectedItems:    10000,
		BloomFPRate:           0.01,
		BloomRotationInterval: 60 * time.Second,

		HealthcheckURL:      "http://localhost:8080/health",
		HealthcheckInterval: 5 * time.Second,

		APIAddr: "0.0.0.0:7947",
	}
}

// Load reads YAML from path into a default-backed struct, then applies env vars on top.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path) //nolint:gosec // config path is user-provided CLI arg
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if err := envconfig.Process("pheme", cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	return cfg, nil
}

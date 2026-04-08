package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the top-level application configuration.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Collectors CollectorConfigs `yaml:"collectors"`
	Storage StorageConfig `yaml:"storage"`
	Pricing PricingConfig `yaml:"pricing"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port        int    `yaml:"port"`
	BindAddress string `yaml:"bind_address"`
}

// CollectorConfigs groups configuration for all data source collectors.
type CollectorConfigs struct {
	Claude   CollectorConfig `yaml:"claude"`
	Codex    CollectorConfig `yaml:"codex"`
	OpenClaw CollectorConfig `yaml:"openclaw"`
	OpenCode CollectorConfig `yaml:"opencode"`
}

// CollectorConfig holds settings for a single data source collector.
type CollectorConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Paths        []string      `yaml:"paths"`
	ScanInterval time.Duration `yaml:"scan_interval"`
}

// StorageConfig holds SQLite database settings.
type StorageConfig struct {
	Path string `yaml:"path"`
}

// PricingConfig holds model pricing sync settings.
type PricingConfig struct {
	SyncInterval time.Duration `yaml:"sync_interval"`
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// DefaultConfig returns a Config with sensible defaults for all fields.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Server: ServerConfig{Port: 9800, BindAddress: "127.0.0.1"},
		Collectors: CollectorConfigs{
			Claude: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".claude", "projects")},
				ScanInterval: 60 * time.Second,
			},
			Codex: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".codex", "sessions")},
				ScanInterval: 60 * time.Second,
			},
			OpenClaw: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".openclaw", "agents")},
				ScanInterval: 60 * time.Second,
			},
			OpenCode: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".local", "share", "opencode", "opencode.db")},
				ScanInterval: 60 * time.Second,
			},
		},
		Storage: StorageConfig{Path: "./agent-usage.db"},
		Pricing: PricingConfig{SyncInterval: time.Hour},
	}
}

// ResolveConfigPath returns the config file path to use, checking in order:
// 1. Explicit path from --config flag (if non-empty)
// 2. /etc/agent-usage/config.yaml (Docker / system-wide)
// 3. ./config.yaml (local default)
func ResolveConfigPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if _, err := os.Stat("/etc/agent-usage/config.yaml"); err == nil {
		return "/etc/agent-usage/config.yaml"
	}
	return "config.yaml"
}

// Load reads configuration from the given YAML file path, falling back to
// defaults for any missing fields. If the file does not exist, defaults are used.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// Expand ~ in paths
	for i, p := range cfg.Collectors.Claude.Paths {
		cfg.Collectors.Claude.Paths[i] = expandPath(p)
	}
	for i, p := range cfg.Collectors.Codex.Paths {
		cfg.Collectors.Codex.Paths[i] = expandPath(p)
	}
	for i, p := range cfg.Collectors.OpenClaw.Paths {
		cfg.Collectors.OpenClaw.Paths[i] = expandPath(p)
	}
	for i, p := range cfg.Collectors.OpenCode.Paths {
		cfg.Collectors.OpenCode.Paths[i] = expandPath(p)
	}
	cfg.Storage.Path = expandPath(cfg.Storage.Path)
	return cfg, nil
}

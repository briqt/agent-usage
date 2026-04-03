package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Collectors CollectorConfigs `yaml:"collectors"`
	Storage StorageConfig `yaml:"storage"`
	Pricing PricingConfig `yaml:"pricing"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type CollectorConfigs struct {
	Claude CollectorConfig `yaml:"claude"`
	Codex  CollectorConfig `yaml:"codex"`
}

type CollectorConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Paths        []string      `yaml:"paths"`
	ScanInterval time.Duration `yaml:"scan_interval"`
}

type StorageConfig struct {
	Path string `yaml:"path"`
}

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

func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Server: ServerConfig{Port: 9800},
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
		},
		Storage: StorageConfig{Path: "./devobs.db"},
		Pricing: PricingConfig{SyncInterval: time.Hour},
	}
}

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
	cfg.Storage.Path = expandPath(cfg.Storage.Path)
	return cfg, nil
}

// Package config manages the bordo CLI configuration stored in ~/.bordo/cli.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the CLI configuration stored on disk.
type Config struct {
	Server string `yaml:"server"`
	Token  string `yaml:"token"`
}

// Default returns the path to the CLI config file.
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bordo", "cli.yaml")
}

// Load reads the CLI config from disk. Returns an empty config if not found.
func Load() (*Config, error) {
	cfg := &Config{Server: "http://localhost:7401"}
	data, err := os.ReadFile(Path())
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cli config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing cli config: %w", err)
	}
	return cfg, nil
}

// Save writes the config to disk.
func Save(cfg *Config) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Set updates a single key in the on-disk config.
func Set(key, value string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	switch key {
	case "server":
		cfg.Server = value
	case "token":
		cfg.Token = value
	default:
		return fmt.Errorf("unknown config key %q (valid: server, token)", key)
	}
	return Save(cfg)
}

// Get retrieves a single key from the on-disk config.
func Get(key string) (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}
	switch key {
	case "server":
		return cfg.Server, nil
	case "token":
		return cfg.Token, nil
	default:
		return "", fmt.Errorf("unknown config key %q (valid: server, token)", key)
	}
}

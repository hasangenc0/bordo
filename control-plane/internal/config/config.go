// Package config loads and validates bordod configuration from YAML and env vars.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level bordod configuration.
type Config struct {
	Server ServerConfig `yaml:"server"`
	Store  StoreConfig  `yaml:"store"`
	Log    LogConfig    `yaml:"log"`
}

// ServerConfig controls the HTTP server.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// StoreConfig controls the state store.
type StoreConfig struct {
	// Driver is "sqlite" or "postgres".
	Driver string `yaml:"driver"`
	// Path is the file path for SQLite or the DSN for Postgres.
	Path string `yaml:"path"`
}

// LogConfig controls logging.
type LogConfig struct {
	// Level is one of "debug", "info", "warn", "error".
	Level string `yaml:"level"`
	// Format is "json" or "text".
	Format string `yaml:"format"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Server: ServerConfig{
			Port: 7401,
		},
		Store: StoreConfig{
			Driver: "sqlite",
			Path:   filepath.Join(home, ".bordo", "bordod.db"),
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load reads configuration from a YAML file and then overlays BORDO_* environment
// variables on top. If path is empty, Load looks for bordod.yaml in the current
// directory, $XDG_CONFIG_HOME/bordo/, and ~/.bordo/ in that order.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path == "" {
		path = findConfigFile()
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config %s: %w", path, err)
		}
	}

	applyEnv(cfg)
	return cfg, nil
}

// findConfigFile searches standard locations for bordod.yaml.
func findConfigFile() string {
	candidates := []string{
		"bordod.yaml",
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "bordo", "bordod.yaml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".bordo", "bordod.yaml"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// applyEnv overlays BORDO_* environment variables onto cfg.
// Supported variables:
//
//	BORDO_SERVER_PORT      int
//	BORDO_STORE_DRIVER     string
//	BORDO_STORE_PATH       string
//	BORDO_LOG_LEVEL        string
//	BORDO_LOG_FORMAT       string
func applyEnv(cfg *Config) {
	if v := os.Getenv("BORDO_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = n
		}
	}
	if v := os.Getenv("BORDO_STORE_DRIVER"); v != "" {
		cfg.Store.Driver = strings.ToLower(v)
	}
	if v := os.Getenv("BORDO_STORE_PATH"); v != "" {
		cfg.Store.Path = v
	}
	if v := os.Getenv("BORDO_LOG_LEVEL"); v != "" {
		cfg.Log.Level = strings.ToLower(v)
	}
	if v := os.Getenv("BORDO_LOG_FORMAT"); v != "" {
		cfg.Log.Format = strings.ToLower(v)
	}
}

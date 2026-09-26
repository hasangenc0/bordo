// Package template implements the Bordo golden-path template engine.
package template

import "gopkg.in/yaml.v3"

// Manifest is the template.yaml contract for a golden-path template directory.
type Manifest struct {
	// Name is the unique template identifier (e.g. "java-web-service").
	Name string `yaml:"name"`
	// Description is a human-readable summary.
	Description string `yaml:"description"`
	// Version is the template version.
	Version string `yaml:"version"`
	// Variables lists the template variables and their defaults.
	Variables []Variable `yaml:"variables"`
	// BinaryPaths are glob patterns for files to copy without template processing.
	BinaryPaths []string `yaml:"binary_paths"`
}

// Variable is a template variable with an optional default value.
type Variable struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
	Required    bool   `yaml:"required"`
}

// ParseManifest reads and parses a template.yaml file.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	return &m, yaml.Unmarshal(data, &m)
}

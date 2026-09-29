// Package template provides the templates discovery API for Bordo.
package template

import "gopkg.in/yaml.v3"

// TemplateVar is a variable declared by a template.
type TemplateVar struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

// TemplateMeta is the metadata parsed from a template.yaml file.
type TemplateMeta struct {
	Name        string        `yaml:"name" json:"name"`
	Description string        `yaml:"description" json:"description"`
	Category    string        `yaml:"category" json:"category"`
	Icon        string        `yaml:"icon" json:"icon"`
	Author      string        `yaml:"author" json:"author"`
	Variables   []TemplateVar `yaml:"variables" json:"variables"`
}

// parseMeta parses a template.yaml YAML byte slice into a TemplateMeta.
func parseMeta(data []byte) (*TemplateMeta, error) {
	var m TemplateMeta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

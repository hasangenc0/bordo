package template

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// Vars is the variable context passed to all template files.
type Vars struct {
	ProjectName  string
	GroupId      string
	BordoVersion string
	CreatedAt    string
	Extra        map[string]string
}

// Engine expands a template directory into a target directory.
type Engine struct {
	// TemplateRoot is the directory containing template directories.
	TemplateRoot string
}

// NewEngine creates an Engine rooted at templateRoot.
func NewEngine(templateRoot string) *Engine {
	return &Engine{TemplateRoot: templateRoot}
}

// Expand reads all files from templateDir and writes expanded copies to targetDir.
// Files matching BinaryPaths in the manifest are copied verbatim.
func (e *Engine) Expand(templateName string, vars Vars, targetDir string) error {
	templateDir := filepath.Join(e.TemplateRoot, templateName)

	manifestData, err := os.ReadFile(filepath.Join(templateDir, "template.yaml"))
	if err != nil {
		return fmt.Errorf("reading template.yaml: %w", err)
	}
	manifest, err := ParseManifest(manifestData)
	if err != nil {
		return fmt.Errorf("parsing template.yaml: %w", err)
	}

	if vars.CreatedAt == "" {
		vars.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	return filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(templateDir, path)
		if err != nil {
			return err
		}

		// Skip the manifest itself.
		if rel == "template.yaml" {
			return nil
		}

		// Expand variable references in the path.
		expandedRel, err := expandPath(rel, vars)
		if err != nil {
			return fmt.Errorf("expanding path %s: %w", rel, err)
		}

		target := filepath.Join(targetDir, expandedRel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if isBinary(rel, manifest.BinaryPaths) {
			return copyFile(path, target)
		}
		return expandFile(path, target, vars)
	})
}

// Validate checks that a template directory has a valid template.yaml.
func (e *Engine) Validate(templateName string) error {
	templateDir := filepath.Join(e.TemplateRoot, templateName)
	data, err := os.ReadFile(filepath.Join(templateDir, "template.yaml"))
	if err != nil {
		return fmt.Errorf("template.yaml not found: %w", err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return fmt.Errorf("invalid template.yaml: %w", err)
	}
	if m.Name == "" {
		return fmt.Errorf("template.yaml: name is required")
	}
	return nil
}

// List returns the names of all available templates under TemplateRoot.
func (e *Engine) List() ([]string, error) {
	entries, err := os.ReadDir(e.TemplateRoot)
	if err != nil {
		return nil, fmt.Errorf("reading templates dir: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.TemplateRoot, entry.Name(), "template.yaml")); err == nil {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// expandPath expands Go template expressions in a file/dir path segment.
func expandPath(path string, vars Vars) (string, error) {
	tmpl, err := template.New("path").Option("missingkey=zero").Parse(path)
	if err != nil {
		return path, nil // non-template path — return as-is
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// expandFile reads src, runs it through text/template with vars, and writes to dst.
func expandFile(src, dst string, vars Vars) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	tmpl, err := template.New(filepath.Base(src)).Option("missingkey=zero").Parse(string(data))
	if err != nil {
		// File uses non-Go-template syntax (e.g. pom.xml with ${...}): copy verbatim.
		return os.WriteFile(dst, data, 0o644)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		// Execution error — fall back to verbatim copy.
		return os.WriteFile(dst, data, 0o644)
	}

	return os.WriteFile(dst, buf.Bytes(), 0o644)
}

// copyFile copies src to dst verbatim, preserving permissions.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// isBinary returns true if path matches any binary_paths glob in the manifest.
func isBinary(path string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}
		// Also check basename.
		matched, err = filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}
	// Treat common binary/non-template extensions as binary.
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".zip", ".jar":
		return true
	}
	return false
}

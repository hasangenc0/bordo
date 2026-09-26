// Package container implements container image building for Bordo.
package container

import "context"

// LogLine is a single line of build output.
type LogLine struct {
	Text string
}

// BuildOptions configures a container image build.
type BuildOptions struct {
	ProjectID   string
	Dockerfile  string // relative path within ContextPath; default: "Dockerfile"
	ContextPath string // local directory containing the build context
	ImageName   string // e.g. "my-api"
	Tag         string // e.g. "abc1234"
	RegistryURL string // e.g. "localhost:5000"
}

// Builder builds a container image from a source directory.
type Builder interface {
	// Build starts a build and returns a channel of log lines.
	// The channel is closed when the build completes or fails.
	// The returned error is non-nil only if the build cannot start.
	Build(ctx context.Context, opts BuildOptions) (<-chan LogLine, error)
}

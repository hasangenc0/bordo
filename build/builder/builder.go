// Package builder exposes container image building for use by other modules.
package builder

import (
	"context"

	"github.com/hasangenc0/bordo/build/internal/container"
)

// LogLine is a single line of build output.
type LogLine = container.LogLine

// BuildOptions configures a container image build.
type BuildOptions = container.BuildOptions

// NewLocalBuilder returns a Builder backed by the local Docker daemon.
func NewLocalBuilder() Builder {
	return container.NewLocalBuilder()
}

// Builder builds a container image from a source directory.
type Builder interface {
	Build(ctx context.Context, opts BuildOptions) (<-chan LogLine, error)
}

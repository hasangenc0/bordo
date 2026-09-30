// Package reconciler is a public entry point for the GitOps reconciler.
package reconciler

import (
	"log/slog"
	"time"

	"github.com/hasangenc0/bordo/release/internal/reconciler"
)

// Reconciler is an alias for the internal reconciler so other modules can use it.
type Reconciler = reconciler.Reconciler

// New creates a Reconciler that polls repoURL every pollInterval and applies changed
// manifests via kubectl.
func New(repoURL, localPath string, pollInterval time.Duration, logger *slog.Logger) *Reconciler {
	return reconciler.New(repoURL, localPath, pollInterval, logger)
}

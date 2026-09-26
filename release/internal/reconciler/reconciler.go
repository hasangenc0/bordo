// Package reconciler watches a desired-state git repo and applies manifests to regional clusters.
package reconciler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Reconciler polls a desired-state git repo and applies changed manifests via kubectl.
type Reconciler struct {
	repoURL      string
	localPath    string
	pollInterval time.Duration
	logger       *slog.Logger

	mu      sync.RWMutex
	regions map[string]string // name → kubeconfig path

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a Reconciler. localPath is where the repo is cloned (~/.bordo/desired-state).
func New(repoURL, localPath string, pollInterval time.Duration, logger *slog.Logger) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{
		repoURL:      repoURL,
		localPath:    localPath,
		pollInterval: pollInterval,
		logger:       logger,
		regions:      make(map[string]string),
		stopCh:       make(chan struct{}),
	}
}

// AddRegion registers a region and its kubeconfig path.
func (r *Reconciler) AddRegion(name, kubeconfigPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.regions[name] = kubeconfigPath
}

// Start begins the polling loop in a background goroutine.
func (r *Reconciler) Start(ctx context.Context) error {
	if r.repoURL == "" {
		r.logger.Warn("reconciler: no repo URL configured, skipping")
		return nil
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(r.pollInterval)
		defer ticker.Stop()

		// Initial reconcile immediately.
		if err := r.reconcileOnce(ctx); err != nil {
			r.logger.Warn("reconciler: initial reconcile failed", "error", err)
		}

		for {
			select {
			case <-ticker.C:
				if err := r.reconcileOnce(ctx); err != nil {
					r.logger.Warn("reconciler: reconcile failed", "error", err)
				}
			case <-r.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop shuts down the reconcile loop.
func (r *Reconciler) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// reconcileOnce pulls the repo and applies any changed manifests.
func (r *Reconciler) reconcileOnce(ctx context.Context) error {
	changed, err := cloneOrPull(ctx, r.repoURL, r.localPath)
	if err != nil {
		return err
	}
	if !changed {
		r.logger.Debug("reconciler: no changes detected")
		return nil
	}

	changedDirs, err := getChangedDirs(ctx, r.localPath)
	if err != nil {
		r.logger.Warn("reconciler: could not get changed dirs, applying all", "error", err)
		changedDirs = nil
	}

	r.mu.RLock()
	regions := make(map[string]string, len(r.regions))
	for k, v := range r.regions {
		regions[k] = v
	}
	r.mu.RUnlock()

	for regionName, kubeconfig := range regions {
		manifestDir := r.localPath + "/" + regionName
		if len(changedDirs) > 0 && !containsPrefix(changedDirs, regionName) {
			continue
		}
		r.logger.Info("reconciler: applying manifests", "region", regionName, "dir", manifestDir)
		if err := kubectlApply(ctx, manifestDir, kubeconfig); err != nil {
			r.logger.Error("reconciler: kubectl apply failed", "region", regionName, "error", err)
			// Continue with other regions.
		}
	}

	return nil
}

func containsPrefix(dirs []string, prefix string) bool {
	for _, d := range dirs {
		if len(d) >= len(prefix) && d[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

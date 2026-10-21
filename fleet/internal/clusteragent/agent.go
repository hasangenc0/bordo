// Package clusteragent implements the Bordo on-cluster reconciler (ADR-0006
// Phase 2). It runs INSIDE a region's k3s cluster as a Deployment, authenticates
// to the control plane with the region's cluster token, PULLS desired state, and
// applies it using the pod's in-cluster RBAC (a ServiceAccount scoped to the
// bordo-apps namespace) — never a cluster-admin kubeconfig held by the control
// plane.
package clusteragent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	defaultInterval  = 30 * time.Second
	defaultNamespace = "bordo-apps"
)

// Config is the reconciler's runtime configuration (from env, set by its
// Deployment manifest).
type Config struct {
	CPURL        string        // BORDO_CP_URL — control-plane URL reachable from the cluster
	ClusterToken string        // BORDO_CLUSTER_TOKEN — region-scoped reconciler credential
	Interval     time.Duration // BORDO_RECONCILE_INTERVAL seconds (default 30)
	Namespace    string        // BORDO_APPS_NAMESPACE (default bordo-apps)
}

// ConfigFromEnv builds and validates a Config from environment variables.
func ConfigFromEnv() (Config, error) {
	c := Config{
		CPURL:        strings.TrimRight(os.Getenv("BORDO_CP_URL"), "/"),
		ClusterToken: os.Getenv("BORDO_CLUSTER_TOKEN"),
		Interval:     defaultInterval,
		Namespace:    defaultNamespace,
	}
	if v := os.Getenv("BORDO_RECONCILE_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Interval = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("BORDO_APPS_NAMESPACE"); v != "" {
		c.Namespace = v
	}
	if c.CPURL == "" {
		return c, errors.New("BORDO_CP_URL is required")
	}
	if c.ClusterToken == "" {
		return c, errors.New("BORDO_CLUSTER_TOKEN is required")
	}
	return c, nil
}

// Applier applies a bundle of Kubernetes manifests. It is injectable so the
// reconcile logic can be unit-tested without a cluster.
type Applier func(ctx context.Context, namespace, manifests string) error

// Agent is the on-cluster reconciler.
type Agent struct {
	cfg   Config
	httpc *http.Client
	apply Applier
}

// New builds an Agent that applies via kubectl using the pod's in-cluster RBAC.
func New(cfg Config) *Agent {
	return &Agent{
		cfg:   cfg,
		httpc: &http.Client{Timeout: 30 * time.Second},
		apply: kubectlApply,
	}
}

// fetchDesiredState pulls the region's manifests from the control plane.
func (a *Agent) fetchDesiredState(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.CPURL+"/fleet/desired-state", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.ClusterToken)
	resp, err := a.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return "", fmt.Errorf("desired-state: %s", e.Error)
	}
	var out struct {
		Region    string `json:"region"`
		Manifests string `json:"manifests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Manifests, nil
}

// ReconcileOnce pulls desired state and applies it. Empty desired state is a
// no-op (nothing to run yet), not an error.
func (a *Agent) ReconcileOnce(ctx context.Context) error {
	manifests, err := a.fetchDesiredState(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(manifests) == "" {
		return nil
	}
	return a.apply(ctx, a.cfg.Namespace, manifests)
}

// Run reconciles immediately, then on every interval until ctx is cancelled. A
// failed cycle is logged and retried next tick rather than killing the loop.
func (a *Agent) Run(ctx context.Context) error {
	fmt.Printf("[bordo-reconciler] starting; pulling desired state from %s every %s\n", a.cfg.CPURL, a.cfg.Interval)
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()
	for {
		if err := a.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
			fmt.Printf("[bordo-reconciler] reconcile error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// kubectlApply pipes manifests to `kubectl apply`. In-cluster, kubectl uses the
// pod's ServiceAccount automatically (no kubeconfig), so RBAC is enforced by the
// cluster, scoped to the namespace the SA is bound to.
func kubectlApply(ctx context.Context, namespace, manifests string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-n", namespace, "-f", "-")
	cmd.Stdin = strings.NewReader(manifests)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply: %w\n%s", err, out.String())
	}
	return nil
}

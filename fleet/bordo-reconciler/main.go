// Command bordo-reconciler is the Bordo on-cluster reconciler (ADR-0006 Phase 2).
// It runs inside a region's k3s cluster, pulls desired state from the control
// plane using the region's cluster token, and applies it with the pod's
// in-cluster RBAC — so the control plane never holds an admin kubeconfig.
//
// Configuration is via environment variables (set by its Deployment manifest):
//
//	BORDO_CP_URL              control-plane URL reachable from the cluster (required)
//	BORDO_CLUSTER_TOKEN       region-scoped reconciler credential (required)
//	BORDO_RECONCILE_INTERVAL  seconds between reconciles (default 30)
//	BORDO_APPS_NAMESPACE      namespace to apply into (default bordo-apps)
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hasangenc0/bordo/fleet/internal/clusteragent"
)

func main() {
	cfg, err := clusteragent.ConfigFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bordo-reconciler: "+err.Error())
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := clusteragent.New(cfg).Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "bordo-reconciler: "+err.Error())
		os.Exit(1)
	}
}

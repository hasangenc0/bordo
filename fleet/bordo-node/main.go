// Command bordo-node is the Bordo node agent (ADR-0006 Phase 1). It runs on a
// target VM, redeems a single-use join token, installs k3s, and registers the
// cluster with the Bordo control plane — all outbound, so the control plane
// never needs SSH access to the VM.
//
// Configuration is via environment variables (see package node):
//
//	BORDO_CP_URL       control-plane base URL reachable from this VM (required)
//	BORDO_JOIN_TOKEN   one-time token from `bordo region add` (required)
//	BORDO_NODE_NAME    node name (default: hostname)
//	BORDO_K3S_VERSION  pinned k3s version to install
//	BORDO_NODE_ADVERTISE_ADDR  address the control plane should use to reach the API server
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hasangenc0/bordo/fleet/internal/node"
)

func main() {
	cfg, err := node.ConfigFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bordo-node: "+err.Error())
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := node.Run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "bordo-node: bootstrap failed: "+err.Error())
		os.Exit(1)
	}
}

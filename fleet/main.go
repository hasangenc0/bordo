// Package main is the entry point for bordo-fleet, the Bordo fleet agent.
//
// bordo-fleet handles the substrate layer of the Bordo platform:
//   - Bootstrapping k3s on target VMs over SSH (BRD-010)
//   - Registering regional clusters with the control plane (BRD-011)
//   - Running the multi-region fleet controller (BRD-012)
//
// In production, the fleet agent runs inside bordod as a goroutine.
// This binary is provided for standalone fleet operations and testing.
//
// See docs/architecture/ADR-0004-k3s-any-vm-substrate.md for rationale.
// See tracker/backlog/BRD-010-k3s-bootstrap-agent.md for the next implementation step.
package main

import (
	"fmt"
	"os"
)

// Version variables are set at build time via -ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("bordo-fleet version %s (commit %s, built %s)\n", version, commit, buildTime)
		return
	}

	// TODO(BRD-010): implement fleet bootstrap agent.
	// TODO(BRD-011): implement region registration.
	// TODO(BRD-012): implement fleet controller.
	fmt.Fprintf(os.Stderr, "bordo-fleet: not yet implemented — see tracker/backlog/BRD-010-k3s-bootstrap-agent.md\n")
	os.Exit(1)
}

// Package main is the entry point for bordod, the Bordo control-plane daemon.
//
// bordod is a single self-hostable binary that exposes a gRPC + REST API for
// the Bordo software factory platform. It hosts:
//   - Project registry & catalog (BRD-002)
//   - Fleet state (regions, clusters, VMs) (BRD-011)
//   - Build orchestration (BRD-007)
//   - Release orchestration (BRD-020)
//   - Observe query facade (BRD-031)
//
// State is stored in an embedded SQLite database (~/.bordo/bordod.db) by default,
// or a Postgres database when configured (see bordod.yaml).
//
// Usage:
//
//	bordod serve                   Start the API server
//	bordod migrate                 Run pending DB migrations and exit
//	bordod version                 Print version and exit
//
// See docs/architecture/ADR-0003-control-plane-single-binary.md for rationale.
// See tracker/todo/BRD-001-control-plane-api-server.md for the next implementation step.
package main

import (
	"fmt"
	"os"
)

// Version variables are set at build time via -ldflags.
// See Makefile build-cp target.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("bordod version %s (commit %s, built %s)\n", version, commit, buildTime)
		return
	}

	// TODO(BRD-001): implement Cobra command tree (serve, migrate, version).
	// For now, print a placeholder so the skeleton compiles and runs.
	fmt.Fprintf(os.Stderr, "bordod: control-plane not yet implemented — see tracker/todo/BRD-001-control-plane-api-server.md\n")
	os.Exit(1)
}

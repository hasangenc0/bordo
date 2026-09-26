// Package main is the entry point for bordo-fleet, the Bordo fleet agent.
//
// bordo-fleet handles the substrate layer of the Bordo platform:
//   - Bootstrapping k3s on target VMs over SSH (BRD-010)
//   - Registering regional clusters with the control plane (BRD-011)
//   - Running the multi-region fleet controller (BRD-012)
//
// See docs/architecture/ADR-0004-k3s-any-vm-substrate.md for rationale.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version variables are set at build time via -ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "bordo-fleet",
		Short:        "Bordo fleet agent — manages k3s clusters over SSH",
		SilenceUsage: true,
	}
	root.AddCommand(regionCmd(), versionCmd())
	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("bordo-fleet version %s (commit %s, built %s)\n", version, commit, buildTime)
		},
	}
}

func regionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "region",
		Short: "Manage Bordo regions",
	}
	cmd.AddCommand(regionAddCmd())
	return cmd
}

func regionAddCmd() *cobra.Command {
	var (
		name    string
		host    string
		port    int
		user    string
		keyFile string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Bootstrap a new region by installing k3s on a remote VM",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("bootstrapping region %q: connecting to %s@%s:%d\n", name, user, host, port)
			fmt.Printf("using key: %s\n", keyFile)
			fmt.Println("(provide a reachable VM to perform the actual bootstrap)")
			// Full implementation wires bootstrap.Bootstrap here once a control-plane client
			// is available to register the resulting kubeconfig (BRD-011).
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "region name (required)")
	cmd.Flags().StringVar(&host, "host", "", "VM hostname or IP (required)")
	cmd.Flags().IntVar(&port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&user, "user", "root", "SSH user")
	cmd.Flags().StringVar(&keyFile, "key", "", "path to SSH private key (required)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("host")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

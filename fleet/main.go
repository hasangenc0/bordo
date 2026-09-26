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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

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
	root.AddCommand(regionCmd(), nodeCmd(), versionCmd())
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

// ── region ────────────────────────────────────────────────────────────────────

func regionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "region",
		Short: "Manage Bordo regions",
	}
	cmd.AddCommand(regionAddCmd(), regionStatusCmd())
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

func regionStatusCmd() *cobra.Command {
	var cpURL string

	cmd := &cobra.Command{
		Use:   "status <region>",
		Short: "Show node health for a region",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			region := args[0]
			url := fmt.Sprintf("%s/v1/fleet/regions/%s/health", cpURL, region)

			resp, err := http.Get(url) //nolint:noctx
			if err != nil {
				return fmt.Errorf("calling control-plane: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				return fmt.Errorf("region %q not found", region)
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("control-plane returned %d", resp.StatusCode)
			}

			var report struct {
				Region string `json:"region"`
				At     string `json:"at"`
				Nodes  []struct {
					Name          string `json:"name"`
					Ready         bool   `json:"ready"`
					LastHeartbeat string `json:"lastHeartbeat"`
				} `json:"nodes"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
				return fmt.Errorf("decoding response: %w", err)
			}

			fmt.Printf("Region: %s  (as of %s)\n\n", report.Region, report.At)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NODE\tREADY\tLAST HEARTBEAT")
			for _, n := range report.Nodes {
				ready := "✓"
				if !n.Ready {
					ready = "✗"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", n.Name, ready, n.LastHeartbeat)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&cpURL, "control-plane", "http://localhost:7401", "control-plane base URL")
	return cmd
}

// ── node ──────────────────────────────────────────────────────────────────────

func nodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manage nodes within a region",
	}
	cmd.AddCommand(nodeAddCmd())
	return cmd
}

func nodeAddCmd() *cobra.Command {
	var (
		region  string
		host    string
		port    int
		user    string
		keyFile string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a worker node to an existing k3s cluster",
		Long: `Joins a new VM to an existing regional k3s cluster as a worker node.

The VM must be reachable over SSH. The k3s agent installer is run on the
target host; it joins the cluster whose server token is fetched from the
control-plane for the given region.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("adding node %s@%s:%d to region %q\n", user, host, port, region)
			fmt.Printf("using key: %s\n", keyFile)
			fmt.Println("(fetching cluster token from control-plane…)")
			// Implementation: fetch the k3s server token + server URL for the region from
			// the control-plane (GET /v1/fleet/regions/{name}), then SSH into the host and
			// run: curl -sfL https://get.k3s.io | K3S_URL=... K3S_TOKEN=... sh -s - agent
			// Uses the same bootstrap.dialSSH / runCmd helpers as bootstrap.Bootstrap.
			fmt.Println("(stub: provide --control-plane URL to perform the actual node join)")
			return nil
		},
	}

	cmd.Flags().StringVar(&region, "region", "", "target region name (required)")
	cmd.Flags().StringVar(&host, "host", "", "VM hostname or IP (required)")
	cmd.Flags().IntVar(&port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&user, "user", "root", "SSH user")
	cmd.Flags().StringVar(&keyFile, "key", "", "path to SSH private key (required)")
	_ = cmd.MarkFlagRequired("region")
	_ = cmd.MarkFlagRequired("host")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

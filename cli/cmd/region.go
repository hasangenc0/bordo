package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/hasangenc0/bordo/cli/internal/client"
	"github.com/hasangenc0/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// RegionCmd returns the `bordo region` command group.
func RegionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "region",
		Short: "Manage fleet regions (k3s clusters)",
	}
	cmd.AddCommand(regionAddCmd(), regionListCmd(), regionTokenCmd(), regionSetHostCmd(), regionUpgradeCmd())
	return cmd
}

func regionAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Register a region and print its node-join command",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewFromConfig()
			if err != nil {
				return err
			}
			name := args[0]
			if _, err := c.CreateRegion(cmd.Context(), name); err != nil {
				return fmt.Errorf("create region: %w", err)
			}
			tok, err := c.MintJoinToken(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("mint join token: %w", err)
			}
			printJoinInstructions(name, tok)
			return nil
		},
	}
}

func regionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "status"},
		Short:   "List regions with their status and disk usage",
		Long: "Shows each region's live status and how full its machine's disk is, as last reported by its\n" +
			"reconciler. A full disk is the usual reason a region turns unreachable.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewFromConfig()
			if err != nil {
				return err
			}
			f, err := c.Fleet(cmd.Context())
			if err != nil {
				return err
			}
			printFleet(os.Stdout, f)
			return nil
		},
	}
}

// printFleet renders the region table plus the platform machine's disk. A region that is
// unreachable or has a filling disk gets its explanation under the row.
func printFleet(w io.Writer, f *client.Fleet) {
	if len(f.Regions) == 0 {
		fmt.Fprintln(w, "No regions yet. Add one with: bordo region add <name>")
	} else {
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "REGION\tSTATUS\tDISK")
		for _, r := range f.Regions {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.Status, describeDisk(r.Disk))
		}
		tw.Flush()
		for _, r := range f.Regions {
			switch {
			case r.Hint != "":
				fmt.Fprintf(w, "\n%s: %s\n", r.Name, r.Hint)
			case r.Disk != nil && r.Disk.Level != "ok":
				fmt.Fprintf(w, "\n%s: the machine's disk is %s. Free space (docker system df -v shows what uses it) before kubelet starts evicting pods.\n", r.Name, r.Disk.Level)
			}
		}
	}
	if f.PlatformDisk != nil {
		fmt.Fprintf(w, "\nBordo server disk: %s\n", describeDisk(f.PlatformDisk))
	}
}

func describeDisk(d *client.DiskUsage) string {
	if d == nil {
		return "unknown (reconciler has not reported; try: bordo region upgrade <name>)"
	}
	s := fmt.Sprintf("%.0f%% used, %s free", d.UsedPercent, humanBytes(uint64(d.FreeBytes)))
	if d.Level != "ok" {
		s += " [" + strings.ToUpper(d.Level) + "]"
	}
	return s
}

func regionTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token <name>",
		Short: "Mint a fresh single-use node-join token for an existing region",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewFromConfig()
			if err != nil {
				return err
			}
			tok, err := c.MintJoinToken(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			printJoinInstructions(args[0], tok)
			return nil
		},
	}
}

func regionSetHostCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-host <name> <ip-or-hostname>",
		Short: "Record the public address of a region's machine (used for NodePort URLs)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewFromConfig()
			if err != nil {
				return err
			}
			if err := c.SetRegionHost(cmd.Context(), args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Region %q is reachable at %s\n", args[0], args[1])
			return nil
		},
	}
}

func regionUpgradeCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "upgrade <name>",
		Short: "Restart a region's on-cluster reconciler onto the latest image",
		Long: "The reconciler runs in the region's cluster and is not part of 'bordo upgrade'. This asks it to\n" +
			"restart itself on its next pull (within about 30s); the cluster starts it again from the latest image.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewFromConfig()
			if err != nil {
				return err
			}
			res, err := c.UpgradeReconciler(cmd.Context(), args[0], force)
			if err != nil {
				return err
			}
			switch {
			case res.RestartRequested:
				fmt.Printf("Asked %q's reconciler to restart; it picks up the latest image within about a minute.\n", args[0])
			case !res.Reconciler.CanSelfUpgrade:
				fmt.Printf("%q's reconciler is too old to restart itself (it reports no revision, or has not pulled yet).\n"+
					"Restart it once on the region's machine:\n  kubectl -n bordo-system rollout restart deployment/bordo-reconciler\n"+
					"After that, 'bordo region upgrade' works.\n", args[0])
			default:
				fmt.Printf("%q's reconciler is already current (revision %d). Use --force to restart it anyway.\n", args[0], res.Reconciler.Revision)
			}
			fmt.Printf("\nRegions joined before the reconciler was hardened keep their old Deployment. To make it survive a\n"+
				"full disk (critical priority, tolerates the pressure taints, has resource requests), run once on the machine:\n  %s\n", hardenReconcilerCmd)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "restart even if the reconciler is current")
	return cmd
}

// hardenReconcilerCmd patches an existing reconciler Deployment to match what new regions get (see
// renderReconcilerManifests in fleet/internal/node). The patch also restarts the pod, onto the
// latest image.
const hardenReconcilerCmd = `kubectl -n bordo-system patch deployment bordo-reconciler -p '{"spec":{"template":{"spec":{"priorityClassName":"system-cluster-critical","tolerations":[{"key":"node.kubernetes.io/disk-pressure","operator":"Exists","effect":"NoSchedule"},{"key":"node.kubernetes.io/memory-pressure","operator":"Exists","effect":"NoSchedule"},{"key":"node.kubernetes.io/pid-pressure","operator":"Exists","effect":"NoSchedule"}],"containers":[{"name":"reconciler","resources":{"requests":{"cpu":"20m","memory":"64Mi","ephemeral-storage":"32Mi"},"limits":{"memory":"256Mi"}}}]}}}}'`

// printJoinInstructions shows the one-time token and the command to run on the
// target VM. The token is single-use and short-lived (ADR-0006 Phase 1): the
// node agent redeems it outbound, so bordod never needs an SSH key.
func printJoinInstructions(region string, tok *client.JoinToken) {
	cpURL := "<control-plane-url>"
	if cfg, err := config.Load(); err == nil && cfg.Server != "" {
		cpURL = cfg.Server
	}
	fmt.Printf("Region %q is registered. One-time join token minted (expires %s).\n\n", region, tok.ExpiresAt)
	fmt.Println("On the target VM, run:")
	fmt.Printf("  curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install-node.sh | \\\n")
	fmt.Printf("    BORDO_CP_URL=%s BORDO_JOIN_TOKEN=%s sh\n\n", cpURL, tok.Token)
	fmt.Println("Notes:")
	fmt.Println("  - The token is single-use and expires soon; mint another with")
	fmt.Printf("    'bordo region token %s'.\n", region)
	fmt.Println("  - If the control-plane URL above is localhost, replace it with an")
	fmt.Println("    address the VM can actually reach.")
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hasangenc0/bordo/cli/internal/client"
	"github.com/hasangenc0/bordo/cli/internal/config"
)

// DeployCmd returns the `bordo deploy` command tree.
func DeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy and manage releases",
	}
	cmd.AddCommand(deployCreateCmd(), deployStatusCmd(), deployRollbackCmd())
	return cmd
}

func deployCreateCmd() *cobra.Command {
	var region, strategy, expose, host string

	cmd := &cobra.Command{
		Use:   "create <project-id> <image-tag>",
		Short: "Deploy a container image to a region",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			switch expose {
			case "", "none", "nodeport", "ingress":
			default:
				return fmt.Errorf("invalid --expose %q (valid: none, nodeport, ingress)", expose)
			}
			if expose == "ingress" && host == "" {
				return fmt.Errorf("--expose ingress requires --host <hostname>")
			}
			c := client.New(cfg.Server, cfg.Token)
			rel, err := c.CreateRelease(cmd.Context(), args[0], args[1], region, strategy, expose, host)
			if err != nil {
				return err
			}
			fmt.Printf("release %s created (status: %s)\n", rel.ID, rel.Status)
			if expose == "nodeport" {
				fmt.Println("exposed via NodePort — once deployed, find the port with:")
				fmt.Println("  kubectl -n bordo-apps get svc")
			} else if expose == "ingress" {
				fmt.Printf("exposed via Ingress at host %q (needs an ingress controller in the cluster)\n", host)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&region, "region", "default", "Target region name")
	cmd.Flags().StringVar(&strategy, "strategy", "rolling", "Rollout strategy: rolling, canary, blue-green")
	cmd.Flags().StringVar(&expose, "expose", "none", "Expose the app: none | nodeport | ingress")
	cmd.Flags().StringVar(&host, "host", "", "Ingress hostname (required for --expose ingress)")
	return cmd
}

func deployStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <release-id>",
		Short: "Show the status of a release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			c := client.New(cfg.Server, cfg.Token)
			rel, err := c.GetRelease(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("ID:        %s\n", rel.ID)
			fmt.Printf("Project:   %s\n", rel.ProjectID)
			fmt.Printf("Image:     %s\n", rel.ImageTag)
			fmt.Printf("Region:    %s\n", rel.Region)
			fmt.Printf("Strategy:  %s\n", rel.Strategy)
			fmt.Printf("Status:    %s\n", rel.Status)
			return nil
		},
	}
}

func deployRollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <release-id>",
		Short: "Roll back a release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			c := client.New(cfg.Server, cfg.Token)
			rel, err := c.RollbackRelease(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("release %s status: %s\n", rel.ID, rel.Status)
			return nil
		},
	}
}

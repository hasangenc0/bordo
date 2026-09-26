package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bordo-io/bordo/cli/internal/client"
	"github.com/bordo-io/bordo/cli/internal/config"
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
	var region, strategy string

	cmd := &cobra.Command{
		Use:   "create <project-id> <image-tag>",
		Short: "Deploy a container image to a region",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			c := client.New(cfg.Server, cfg.Token)
			rel, err := c.CreateRelease(cmd.Context(), args[0], args[1], region, strategy)
			if err != nil {
				return err
			}
			fmt.Printf("release %s created (status: %s)\n", rel.ID, rel.Status)
			return nil
		},
	}
	cmd.Flags().StringVar(&region, "region", "default", "Target region name")
	cmd.Flags().StringVar(&strategy, "strategy", "rolling", "Rollout strategy: rolling, canary, blue-green")
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

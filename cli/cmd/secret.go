package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// SecretCmd returns the `bordo secret` command tree.
func SecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage project secrets",
	}
	cmd.AddCommand(secretSetCmd(), secretListCmd(), secretDeleteCmd())
	return cmd
}

func secretSetCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Store an encrypted secret for a project",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			return c.SetSecret(context.Background(), project, args[0], args[1])
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Project ID")
	return cmd
}

func secretListCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secret keys for a project (values are never shown)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			secs, err := c.ListSecrets(context.Background(), project)
			if err != nil {
				return err
			}
			if len(secs) == 0 {
				fmt.Println("No secrets found.")
				return nil
			}
			for _, s := range secs {
				fmt.Printf("%-30s  created %s\n", s.Key, s.CreatedAt)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Project ID")
	return cmd
}

func secretDeleteCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "delete <key>",
		Short: "Delete a secret from a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			c, err := newClient()
			if err != nil {
				return err
			}
			return c.DeleteSecret(context.Background(), project, args[0])
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Project ID")
	return cmd
}

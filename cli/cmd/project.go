package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"os"

	"github.com/hasangenc0/bordo/cli/internal/client"
	"github.com/hasangenc0/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// ProjectCmd returns the project command group.
func ProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Bordo projects",
	}
	cmd.AddCommand(
		projectCreateCmd(),
		projectListCmd(),
		projectGetCmd(),
		projectDeleteCmd(),
	)
	return cmd
}

func newClient() (*client.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return client.New(cfg.Server, cfg.Token), nil
}

func projectCreateCmd() *cobra.Command {
	var tmpl string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			p, err := c.CreateProject(cmd.Context(), args[0], tmpl)
			if err != nil {
				return err
			}
			fmt.Printf("created project %s (id: %s)\n", p.Name, p.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&tmpl, "template", "", "template name (e.g. java-web-service)")
	return cmd
}

func projectListCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			projects, err := c.ListProjects(cmd.Context())
			if err != nil {
				return err
			}

			if outputFmt == "json" {
				return json.NewEncoder(os.Stdout).Encode(projects)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tTEMPLATE\tSTATUS")
			fmt.Fprintln(w, strings.Repeat("-", 60))
			for _, p := range projects {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.ID[:8], p.Name, p.Template, p.Status)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&outputFmt, "output", "", "output format: json")
	return cmd
}

func projectGetCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "get <name-or-id>",
		Short: "Get project details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			// Try by ID first; if not found, search by name in the list.
			p, err := c.GetProject(cmd.Context(), args[0])
			if err != nil {
				// Fall back: list and find by name.
				all, listErr := c.ListProjects(cmd.Context())
				if listErr != nil {
					return err
				}
				for _, proj := range all {
					if proj.Name == args[0] {
						p = proj
						err = nil
						break
					}
				}
				if err != nil {
					return fmt.Errorf("project %q not found", args[0])
				}
			}

			if outputFmt == "json" {
				return json.NewEncoder(os.Stdout).Encode(p)
			}
			fmt.Printf("id:         %s\n", p.ID)
			fmt.Printf("name:       %s\n", p.Name)
			fmt.Printf("template:   %s\n", p.Template)
			fmt.Printf("status:     %s\n", p.Status)
			fmt.Printf("git_repo:   %s\n", p.GitRepoURL)
			fmt.Printf("created_at: %s\n", p.CreatedAt)
			return nil
		},
	}
	cmd.Flags().StringVar(&outputFmt, "output", "", "output format: json")
	return cmd
}

func projectDeleteCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			// Resolve name → ID.
			id := args[0]
			all, err := c.ListProjects(cmd.Context())
			if err != nil {
				return err
			}
			for _, p := range all {
				if p.Name == args[0] {
					id = p.ID
					break
				}
			}

			if !yes {
				fmt.Printf("delete project %q? [y/N] ", args[0])
				var answer string
				fmt.Scanln(&answer)
				if answer != "y" && answer != "Y" {
					fmt.Println("aborted")
					return nil
				}
			}

			if err := c.DeleteProject(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Printf("deleted project %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation prompt")
	return cmd
}

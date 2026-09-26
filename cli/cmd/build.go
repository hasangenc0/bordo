package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// BuildCmd returns the build command group.
func BuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Manage container builds",
	}
	cmd.AddCommand(buildTriggerCmd(), buildListCmd(), buildLogsCmd())
	return cmd
}

func resolveProjectID(c interface{ ListProjects(interface{}) ([]*struct{ Name, ID string }, error) }, ctx interface{}, nameOrID string) (string, error) {
	return nameOrID, nil // simplified — used below inline
}

func buildTriggerCmd() *cobra.Command {
	var (
		registry string
		tag      string
	)

	cmd := &cobra.Command{
		Use:   "trigger <project-name>",
		Short: "Trigger a container build for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			// Resolve project name → ID.
			projects, err := c.ListProjects(cmd.Context())
			if err != nil {
				return err
			}
			var projectID string
			for _, p := range projects {
				if p.Name == args[0] || p.ID == args[0] {
					projectID = p.ID
					break
				}
			}
			if projectID == "" {
				return fmt.Errorf("project %q not found", args[0])
			}

			if tag == "" {
				tag = "latest"
			}

			b, err := c.TriggerBuild(cmd.Context(), projectID, args[0], tag, registry)
			if err != nil {
				return err
			}
			fmt.Printf("build triggered: %s (status: %s)\n", b.ID[:8], b.Status)
			return nil
		},
	}

	cmd.Flags().StringVar(&registry, "registry", "localhost:5000", "OCI registry URL")
	cmd.Flags().StringVar(&tag, "tag", "", "image tag (default: latest)")
	return cmd
}

func buildListCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "list <project-name>",
		Short: "List builds for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			projects, err := c.ListProjects(cmd.Context())
			if err != nil {
				return err
			}
			var projectID string
			for _, p := range projects {
				if p.Name == args[0] || p.ID == args[0] {
					projectID = p.ID
					break
				}
			}
			if projectID == "" {
				return fmt.Errorf("project %q not found", args[0])
			}

			builds, err := c.ListBuilds(cmd.Context(), projectID)
			if err != nil {
				return err
			}

			if outputFmt == "json" {
				return json.NewEncoder(os.Stdout).Encode(builds)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATUS\tIMAGE\tTAG")
			fmt.Fprintln(w, strings.Repeat("-", 60))
			for _, b := range builds {
				id := b.ID
				if len(id) > 8 {
					id = id[:8]
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", id, b.Status, b.ImageName, b.ImageTag)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&outputFmt, "output", "", "output format: json")
	return cmd
}

func buildLogsCmd() *cobra.Command {
	var buildID string

	cmd := &cobra.Command{
		Use:   "logs <project-name>",
		Short: "Show logs for the latest (or specified) build",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}

			if buildID == "" {
				projects, err := c.ListProjects(cmd.Context())
				if err != nil {
					return err
				}
				var projectID string
				for _, p := range projects {
					if p.Name == args[0] || p.ID == args[0] {
						projectID = p.ID
						break
					}
				}
				if projectID == "" {
					return fmt.Errorf("project %q not found", args[0])
				}
				builds, err := c.ListBuilds(cmd.Context(), projectID)
				if err != nil {
					return err
				}
				if len(builds) == 0 {
					return fmt.Errorf("no builds found for project %q", args[0])
				}
				buildID = builds[0].ID
			}

			lines, err := c.GetBuildLogs(cmd.Context(), buildID)
			if err != nil {
				return err
			}
			for _, line := range lines {
				fmt.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&buildID, "build-id", "", "specific build ID (default: latest)")
	return cmd
}

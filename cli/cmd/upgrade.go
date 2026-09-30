package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// UpgradeCmd returns the `bordo upgrade` command.
func UpgradeCmd() *cobra.Command {
	var composeDir string

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Pull the latest Bordo images and restart services",
		Long: `Pull the latest pre-built Bordo images from GHCR and restart all
services in place. Running services are restarted one at a time; user
data (volumes) and configuration are never modified.

The command must be run on the machine where Bordo is installed, or
you can pass --dir to point at the deploy/bordo directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveComposeDir(composeDir)
			if err != nil {
				return err
			}

			fmt.Println("Pulling latest Bordo images...")
			if err := runCompose(dir, "pull"); err != nil {
				return fmt.Errorf("docker compose pull: %w", err)
			}

			fmt.Println("Restarting services...")
			if err := runCompose(dir, "up", "-d", "--remove-orphans"); err != nil {
				return fmt.Errorf("docker compose up: %w", err)
			}

			fmt.Println("Upgrade complete. Check logs with:")
			fmt.Printf("  docker compose -f %s/docker-compose.yml logs -f\n", dir)
			return nil
		},
	}

	cmd.Flags().StringVar(&composeDir, "dir", "", "path to the deploy/bordo directory (default: auto-detect)")
	return cmd
}

// resolveComposeDir returns the directory containing docker-compose.yml.
// It checks --dir, then $BORDO_COMPOSE_DIR, then a set of common install paths.
func resolveComposeDir(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if v := os.Getenv("BORDO_COMPOSE_DIR"); v != "" {
		return v, nil
	}
	candidates := []string{
		"/opt/bordo/deploy/bordo",
		filepath.Join(os.Getenv("HOME"), "bordo/deploy/bordo"),
		"deploy/bordo",
		".",
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "docker-compose.yml")); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find docker-compose.yml — pass --dir /path/to/deploy/bordo")
}

func runCompose(dir string, args ...string) error {
	full := append([]string{"compose", "-f", filepath.Join(dir, "docker-compose.yml")}, args...)
	c := exec.Command("docker", full...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

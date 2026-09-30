package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// UpgradeCmd returns the top-level `bordo upgrade` — a convenience alias for
// `bordo platform upgrade`. It pulls the latest images and restarts services
// without touching data volumes.
func UpgradeCmd() *cobra.Command {
	var composeDir string
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Pull the latest Bordo images and restart services",
		Long: `Pull the latest pre-built Bordo images and restart all services in
place. User data (volumes) and configuration are never modified.

Alias for 'bordo platform upgrade'. Uses ~/.bordo/deploy by default,
or pass --dir for a manually managed compose directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveComposeDir(composeDir)
			if err != nil {
				return err
			}
			fmt.Println("Pulling latest Bordo images...")
			if err := composeInDir(dir, "pull").Run(); err != nil {
				return fmt.Errorf("docker compose pull: %w", err)
			}
			fmt.Println("Restarting services...")
			if err := composeInDir(dir, "up", "-d", "--remove-orphans").Run(); err != nil {
				return fmt.Errorf("docker compose up: %w", err)
			}
			fmt.Println("Upgrade complete.")
			return nil
		},
	}
	cmd.Flags().StringVar(&composeDir, "dir", "", "path to a compose directory (default: ~/.bordo/deploy, then common install paths)")
	return cmd
}

// resolveComposeDir finds the directory holding docker-compose.yml: --dir,
// then $BORDO_COMPOSE_DIR, then the CLI-managed dir and common install paths.
func resolveComposeDir(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if v := os.Getenv("BORDO_COMPOSE_DIR"); v != "" {
		return v, nil
	}
	candidates := []string{
		platformDir(),
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
	return "", fmt.Errorf("no docker-compose.yml found — run 'bordo platform install' or pass --dir")
}

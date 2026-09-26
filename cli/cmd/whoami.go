package cmd

import (
	"fmt"

	"github.com/bordo-io/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// WhoamiCmd prints the current server and auth token status.
func WhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current server and auth token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "server: %s\n", cfg.Server)
			if cfg.Token == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "token:  (not set — run: bordo login)")
			} else {
				masked := maskToken(cfg.Token)
				fmt.Fprintf(cmd.OutOrStdout(), "token:  %s\n", masked)
			}
			return nil
		},
	}
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "****" + t[len(t)-4:]
}

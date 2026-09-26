package cmd

import (
	"fmt"

	"github.com/bordo-io/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// LoginCmd stores the server URL and bearer token.
func LoginCmd() *cobra.Command {
	var (
		server string
		token  string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to a Bordo control plane",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if server != "" {
				cfg.Server = server
			}
			if token != "" {
				cfg.Token = token
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("logged in to %s\n", cfg.Server)
			return nil
		},
	}

	cmd.Flags().StringVar(&server, "server", "", "control-plane URL (e.g. http://localhost:7401)")
	cmd.Flags().StringVar(&token, "token", "", "bearer token")
	return cmd
}

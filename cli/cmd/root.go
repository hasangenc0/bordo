// Package cmd contains all bordo CLI subcommands.
package cmd

import (
	"os"

	"github.com/hasangenc0/bordo/cli/internal/cfaccess"
	"github.com/hasangenc0/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// installCFAccess sends the Cloudflare Access service token (env, else the CLI config) to server.
// It is a no-op unless a token is set and the server is an https URL.
func installCFAccess(server string) {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}
	id, secret := cfg.CFAccessClientID, cfg.CFAccessClientSecret
	if v := os.Getenv(cfaccess.EnvClientID); v != "" {
		id = v
	}
	if v := os.Getenv(cfaccess.EnvClientSecret); v != "" {
		secret = v
	}
	cfaccess.Install(server, id, secret)
}

// Execute is the entry point for the CLI.
func Execute() error {
	return Root().Execute()
}

// Root builds and returns the root cobra command.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:          "bordo",
		Short:        "Bordo software factory CLI",
		SilenceUsage: true,
		// The control plane may be published behind Cloudflare Access; present the machine token to it.
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cfg, err := config.Load()
			if err != nil {
				return
			}
			server := cfg.Server
			if v := os.Getenv("BORDO_SERVER"); v != "" {
				server = v
			}
			installCFAccess(server)
		},
	}

	root.AddCommand(
		VersionCmd(),
		ConfigCmd(),
		LoginCmd(),
		WhoamiCmd(),
		SetupCmd(),
		ProjectCmd(),
		RegionCmd(),
		ApprovalCmd(),
		TemplateCmd(),
		BuildCmd(),
		SecretCmd(),
		DeployCmd(),
		ObserveCmd(),
		AgentCmd(),
		GithubCmd(),
		McpCmd(),
		PlatformCmd(),
		UpgradeCmd(),
	)

	return root
}

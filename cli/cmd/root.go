// Package cmd contains all bordo CLI subcommands.
package cmd

import (
	"github.com/spf13/cobra"
)

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
	}

	configCmd := ConfigCmd()
	configCmd.AddCommand(ConfigSetCmd())
	root.AddCommand(
		VersionCmd(),
		configCmd,
		LoginCmd(),
		WhoamiCmd(),
		SetupCmd(),
		ProjectCmd(),
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

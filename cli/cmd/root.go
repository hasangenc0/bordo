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

	root.AddCommand(
		VersionCmd(),
		ConfigCmd(),
		LoginCmd(),
		WhoamiCmd(),
		ProjectCmd(),
		TemplateCmd(),
		BuildCmd(),
		SecretCmd(),
		DeployCmd(),
		ObserveCmd(),
	)

	return root
}

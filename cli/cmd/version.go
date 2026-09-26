package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version variables set via -ldflags at build time.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

// VersionCmd prints version information and exits.
func VersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("bordo version %s (commit %s, built %s)\n", Version, Commit, BuildTime)
		},
	}
}

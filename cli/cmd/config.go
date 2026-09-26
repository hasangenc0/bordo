package cmd

import (
	"fmt"

	"github.com/bordo-io/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// ConfigCmd returns the config command group.
func ConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
	}
	cmd.AddCommand(configSetCmd(), configGetCmd())
	return cmd
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Set(args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("set %s = %s\n", args[0], args[1])
			return nil
		},
	}
}

func configGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			val, err := config.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Println(val)
			return nil
		},
	}
}

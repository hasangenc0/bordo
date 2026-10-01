package cmd

import (
	"fmt"

	"github.com/hasangenc0/bordo/cli/internal/config"
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
		Short: "Set a configuration value (local CLI config or server setting)",
		Long: `Set a configuration value.

Local CLI keys:    server, token, agent_url, agent_token
Server settings:   deepseek_api_key, base_url, llm_model, llm_url, registry_url
                   (stored on the Bordo server; requires login)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			if serverSettingKeys[key] {
				if err := setServerSetting(key, value); err != nil {
					return err
				}
				fmt.Printf("updated server setting %s\n", key)
				return nil
			}
			if err := config.Set(key, value); err != nil {
				return err
			}
			fmt.Printf("set %s = %s\n", key, value)
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

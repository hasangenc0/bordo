package cmd

import (
	"fmt"
	"os"

	bordobuild "github.com/hasangenc0/bordo/build/template"
	"github.com/spf13/cobra"
)

// TemplateCmd returns the template command group.
func TemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage golden-path templates",
	}
	cmd.AddCommand(templateListCmd(), templateExpandCmd(), templateValidateCmd())
	return cmd
}

func templateRoot() string {
	if r := os.Getenv("BORDO_TEMPLATES"); r != "" {
		return r
	}
	// Default: templates/ relative to the repo root (for local dev) or /usr/share/bordo/templates.
	if _, err := os.Stat("templates"); err == nil {
		return "templates"
	}
	return "/usr/share/bordo/templates"
}

func templateListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available golden-path templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine := bordobuild.NewEngine(templateRoot())
			names, err := engine.List()
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Println("no templates found")
				return nil
			}
			for _, n := range names {
				fmt.Println(n)
			}
			return nil
		},
	}
}

func templateExpandCmd() *cobra.Command {
	var (
		vars   []string
		output string
	)

	cmd := &cobra.Command{
		Use:   "expand <template-name>",
		Short: "Expand a golden-path template into a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine := bordobuild.NewEngine(templateRoot())

			v := bordobuild.Vars{Extra: make(map[string]string)}
			for _, kv := range vars {
				parts := splitKV(kv)
				if len(parts) != 2 {
					return fmt.Errorf("invalid var %q (expected Key=Value)", kv)
				}
				switch parts[0] {
				case "ProjectName":
					v.ProjectName = parts[1]
				case "GroupId":
					v.GroupId = parts[1]
				case "BordoVersion":
					v.BordoVersion = parts[1]
				default:
					v.Extra[parts[0]] = parts[1]
				}
			}

			if output == "" {
				if v.ProjectName != "" {
					output = v.ProjectName
				} else {
					output = args[0] + "-output"
				}
			}

			if err := engine.Expand(args[0], v, output); err != nil {
				return err
			}
			fmt.Printf("expanded template %q → %s\n", args[0], output)
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&vars, "var", nil, "template variable (Key=Value, repeatable)")
	cmd.Flags().StringVar(&output, "output", "", "output directory (default: ProjectName or <template>-output)")
	return cmd
}

func templateValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <template-name-or-dir>",
		Short: "Validate a template's template.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine := bordobuild.NewEngine(templateRoot())
			if err := engine.Validate(args[0]); err != nil {
				return err
			}
			fmt.Printf("template %q is valid\n", args[0])
			return nil
		},
	}
}

func splitKV(s string) []string {
	for i, c := range s {
		if c == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

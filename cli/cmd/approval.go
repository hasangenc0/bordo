package cmd

import (
	"fmt"
	"strings"

	"github.com/hasangenc0/bordo/cli/internal/client"
	"github.com/spf13/cobra"
)

// ApprovalCmd returns the `bordo approval` command group. It decides requests raised outside a
// chat, such as a deploy-on-merge waiting for the go-ahead.
func ApprovalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approval",
		Short: "List and decide pending approvals (e.g. deploy-on-merge)",
	}
	cmd.AddCommand(approvalListCmd(), approvalDecideCmd("approve"), approvalDecideCmd("reject"))
	return cmd
}

func approvalListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show approvals waiting for a decision",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewFromConfig()
			if err != nil {
				return err
			}
			items, err := c.PendingApprovals(cmd.Context())
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("Nothing is waiting for approval.")
				return nil
			}
			for _, a := range items {
				fmt.Printf("%s  %-8s %s\n", a.ID, a.Kind, a.Target)
			}
			fmt.Println("\nDecide with: bordo approval approve <id>   or   bordo approval reject <id>")
			return nil
		},
	}
}

func approvalDecideCmd(verdict string) *cobra.Command {
	return &cobra.Command{
		Use:   verdict + " <id>",
		Short: strings.ToUpper(verdict[:1]) + verdict[1:] + " a pending request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewFromConfig()
			if err != nil {
				return err
			}
			if err := c.DecideApproval(cmd.Context(), args[0], verdict); err != nil {
				return err
			}
			fmt.Printf("%sd %s\n", verdict, args[0])
			return nil
		},
	}
}

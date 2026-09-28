package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/bordo-io/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// GithubCmd returns the github command group.
func GithubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "github",
		Short: "Manage GitHub App integration",
	}
	cmd.AddCommand(githubSetupCmd(), githubInstallCmd(), githubStatusCmd())
	return cmd
}

func githubSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Register a GitHub App for Bordo (opens browser)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			setupURL := cfg.Server + "/github/app/setup"
			fmt.Printf("Opening: %s\n", setupURL)
			if err := openBrowser(setupURL); err != nil {
				fmt.Println("Could not open browser automatically.")
				fmt.Printf("Visit this URL to register the GitHub App:\n  %s\n", setupURL)
			}
			return nil
		},
	}
}

func githubInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the registered GitHub App on your account/org (opens browser)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			installURL := cfg.Server + "/github/app/install"
			fmt.Printf("Opening: %s\n", installURL)
			if err := openBrowser(installURL); err != nil {
				fmt.Println("Could not open browser automatically.")
				fmt.Printf("Visit this URL to install the GitHub App:\n  %s\n", installURL)
			}
			return nil
		},
	}
}

func githubStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show GitHub App integration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			raw, err := c.GetRaw(cmd.Context(), "/v1/github/status")
			if err != nil {
				return err
			}
			var status struct {
				AppRegistered  bool   `json:"app_registered"`
				AppID          int64  `json:"app_id"`
				AppSlug        string `json:"app_slug"`
				InstallationID int64  `json:"installation_id"`
				UserAuthorized bool   `json:"user_authorized"`
			}
			if err := json.Unmarshal(raw, &status); err != nil {
				return err
			}

			check := func(ok bool) string {
				if ok {
					return "✓"
				}
				return "✗"
			}

			fmt.Printf("GitHub App registered:  %s", check(status.AppRegistered))
			if status.AppRegistered {
				fmt.Printf("  (id=%d, slug=%s)", status.AppID, status.AppSlug)
			}
			fmt.Println()
			fmt.Printf("App installed:          %s", check(status.InstallationID != 0))
			if status.InstallationID != 0 {
				fmt.Printf("  (installation_id=%d)", status.InstallationID)
			}
			fmt.Println()
			fmt.Printf("User OAuth authorized:  %s\n", check(status.UserAuthorized))

			if !status.AppRegistered {
				fmt.Println("\nNext step: run `bordo github setup` to register a GitHub App.")
			} else if status.InstallationID == 0 {
				fmt.Println("\nNext step: run `bordo github install` to install the app on your account.")
			}
			return nil
		},
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

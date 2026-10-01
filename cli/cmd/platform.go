package cmd

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hasangenc0/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

//go:embed assets/docker-compose.yml
var composeYAML []byte

// platformDir is where the CLI writes the compose file and .env.
func platformDir() string {
	return filepath.Join(os.Getenv("HOME"), ".bordo", "deploy")
}

// PlatformCmd returns the `bordo platform` command group — the single control
// surface for the Bordo platform services. Docker runs under the hood.
func PlatformCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "platform",
		Short: "Manage the Bordo platform services (install, start, stop, logs)",
		Long: `Manage the Bordo platform lifecycle with a single binary.

Docker is the runtime under the hood, but you never type a docker command.
Typical flow:

  bordo platform install    # write config + pull images
  bordo platform up         # start everything
  bordo setup               # first-run configuration
  bordo platform status     # check health`,
	}
	cmd.AddCommand(
		platformInstallCmd(),
		platformUpCmd(),
		platformDownCmd(),
		platformRestartCmd(),
		platformStatusCmd(),
		platformLogsCmd(),
		platformTokenCmd(),
		platformUpgradeCmd(),
	)
	return cmd
}

func platformInstallCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Write platform config to ~/.bordo/deploy and pull images",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := exec.LookPath("docker"); err != nil {
				return fmt.Errorf("Docker is not installed. Install it first: https://docs.docker.com/engine/install/")
			}

			dir := platformDir()
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return fmt.Errorf("creating %s: %w", dir, err)
			}

			composePath := filepath.Join(dir, "docker-compose.yml")
			if err := os.WriteFile(composePath, composeYAML, 0o644); err != nil {
				return fmt.Errorf("writing compose file: %w", err)
			}
			fmt.Printf("Wrote %s\n", composePath)

			envPath := filepath.Join(dir, ".env")
			if _, err := os.Stat(envPath); os.IsNotExist(err) || force {
				token, err := randomToken()
				if err != nil {
					return err
				}
				env := fmt.Sprintf(
					"# Bordo platform config — edit ports here; keep the token secret.\n"+
						"BORDO_INTERNAL_TOKEN=%s\n"+
						"BORDOD_PORT=7401\n"+
						"AGENT_PORT=7402\n"+
						"CONSOLE_PORT=3000\n",
					token,
				)
				if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
					return fmt.Errorf("writing .env: %w", err)
				}
				fmt.Printf("Generated %s (internal token, ports)\n", envPath)
			} else {
				fmt.Printf("Kept existing %s\n", envPath)
			}

			// Sync the internal token into the CLI config so 'bordo agent chat'
			// can authenticate to the agent directly.
			if tok := readEnvValue(envPath, "BORDO_INTERNAL_TOKEN"); tok != "" {
				if err := config.Set("agent_token", tok); err != nil {
					fmt.Printf("Warning: could not save agent token to CLI config: %v\n", err)
				}
			}

			fmt.Println("Pulling images...")
			if err := compose("pull").Run(); err != nil {
				return fmt.Errorf("docker compose pull: %w", err)
			}

			fmt.Println("\nInstalled. Start with:  bordo platform up")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "regenerate .env even if it exists (rotates the internal token)")
	return cmd
}

func platformUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Start all Bordo services in the background",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			if err := compose("up", "-d", "--remove-orphans").Run(); err != nil {
				return fmt.Errorf("docker compose up: %w", err)
			}
			fmt.Println("\nBordo is up. Next:")
			fmt.Println("  bordo platform token                      # get the setup token")
			fmt.Println("  bordo setup --server http://localhost:7401")
			return nil
		},
	}
}

func platformDownCmd() *cobra.Command {
	var volumes bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop and remove Bordo service containers (data volumes kept)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			composeArgs := []string{"down"}
			if volumes {
				composeArgs = append(composeArgs, "--volumes")
			}
			return compose(composeArgs...).Run()
		},
	}
	cmd.Flags().BoolVar(&volumes, "volumes", false, "also delete data volumes — DESTROYS all Bordo data")
	return cmd
}

func platformRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [service]",
		Short: "Restart all services, or a single named service",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			return compose(append([]string{"restart"}, args...)...).Run()
		},
	}
}

func platformStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			return compose("ps").Run()
		},
	}
}

func platformLogsCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show service logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			composeArgs := []string{"logs", "--tail", "200"}
			if follow {
				composeArgs = append(composeArgs, "-f")
			}
			return compose(append(composeArgs, args...)...).Run()
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream logs")
	return cmd
}

func platformTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Print the one-time setup token (needed for 'bordo setup')",
		Long: `Print bordod's current setup token so you can run 'bordo setup'.

The token is regenerated every time bordod restarts, so this always
returns the token for the latest start. Once setup is complete the token
is no longer logged and this reports that Bordo is already configured.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			dir := platformDir()
			out, err := exec.Command("docker", "compose",
				"-f", filepath.Join(dir, "docker-compose.yml"),
				"logs", "bordod").CombinedOutput()
			if err != nil {
				return fmt.Errorf("reading bordod logs: %w", err)
			}
			tok := extractSetupToken(string(out))
			if tok == "" {
				return fmt.Errorf("no setup token found — Bordo may already be configured, or bordod hasn't started yet (check: bordo platform status)")
			}
			fmt.Println(tok)
			return nil
		},
	}
}

// setupTokenRE matches the 64-hex setup token in both JSON and text log
// formats: `"setup_token":"<hex>"` or `setup_token=<hex>`.
var setupTokenRE = regexp.MustCompile(`setup_token["=:\s]+"?([a-f0-9]{64})`)

// extractSetupToken returns the most recent setup token found in the logs.
// bordod regenerates the token on every start, so the last match is current.
func extractSetupToken(logs string) string {
	matches := setupTokenRE.FindAllStringSubmatch(logs, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}

func platformUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Pull the latest images and restart services (data preserved)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			fmt.Println("Pulling latest images...")
			if err := compose("pull").Run(); err != nil {
				return fmt.Errorf("docker compose pull: %w", err)
			}
			fmt.Println("Restarting...")
			if err := compose("up", "-d", "--remove-orphans").Run(); err != nil {
				return fmt.Errorf("docker compose up: %w", err)
			}
			fmt.Println("Upgrade complete.")
			return nil
		},
	}
}

// compose runs `docker compose` scoped to the CLI-managed platform dir.
func compose(args ...string) *exec.Cmd {
	return composeInDir(platformDir(), args...)
}

// composeInDir runs `docker compose` against the compose file in dir, with the
// working directory set so the .env file is picked up automatically.
func composeInDir(dir string, args ...string) *exec.Cmd {
	full := append([]string{"compose", "-f", filepath.Join(dir, "docker-compose.yml")}, args...)
	c := exec.Command("docker", full...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c
}

// readEnvValue returns the value of key from a KEY=VALUE .env file, or "".
func readEnvValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+"="))
		}
	}
	return ""
}

func ensureInstalled() error {
	if _, err := os.Stat(filepath.Join(platformDir(), "docker-compose.yml")); err != nil {
		return fmt.Errorf("platform not installed — run: bordo platform install")
	}
	return nil
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

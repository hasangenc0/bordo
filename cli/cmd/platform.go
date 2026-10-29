package cmd

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
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

  bordo setup               # does everything below, then connects GitHub
  bordo platform status     # check health

Or step by step:

  bordo platform install    # write config + pull images
  bordo platform up         # start everything
  bordo platform token      # print the one-time setup token
  bordo setup               # first-run configuration`,
	}
	cmd.AddCommand(
		platformInstallCmd(),
		platformUpCmd(),
		platformDownCmd(),
		platformRestartCmd(),
		platformStatusCmd(),
		platformLogsCmd(),
		platformTokenCmd(),
		platformPasswordCmd(),
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
			return runPlatformInstall(force, false)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "regenerate .env even if it exists (rotates the internal token)")
	return cmd
}

// runPlatformInstall writes the compose file and .env and pulls images. quiet
// suppresses the "start with" hint for callers (like `bordo setup`) that go on
// to start the platform themselves.
func runPlatformInstall(force, quiet bool) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("Docker is not installed. Install it first: https://docs.docker.com/engine/install/")
	}

	if desc, conflict := conflictingStack(); conflict {
		fmt.Printf("Warning: another Bordo stack appears to be running: %s.\n"+
			"It would clash on ports 7401/7402/3000, so 'bordo platform up' will\n"+
			"refuse to start until that stack is stopped (or run 'up --force').\n\n", desc)
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
		consolePW, err := randomToken()
		if err != nil {
			return err
		}
		env := fmt.Sprintf(
			"# Bordo platform config — edit ports here; keep the secrets secret.\n"+
				"BORDO_INTERNAL_TOKEN=%s\n"+
				"# Login for the web console (username: bordo).\n"+
				"BORDO_CONSOLE_PASSWORD=%s\n"+
				"BORDOD_PORT=7401\n"+
				"AGENT_PORT=7402\n"+
				"CONSOLE_PORT=3000\n",
			token, consolePW,
		)
		if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
			return fmt.Errorf("writing .env: %w", err)
		}
		fmt.Printf("Generated %s (internal token, console password, ports)\n", envPath)
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

	if !quiet {
		fmt.Println("\nInstalled. Start with:  bordo platform up")
	}
	return nil
}

func platformUpCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start all Bordo services in the background",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlatformUp(force, false)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "start even if another Bordo stack is detected (advanced)")
	return cmd
}

// runPlatformUp starts the compose stack. quiet suppresses the "Next:" hints
// for callers (like `bordo setup`) that continue the flow themselves.
func runPlatformUp(force, quiet bool) error {
	if err := ensureInstalled(); err != nil {
		return err
	}
	if !force {
		if desc, conflict := conflictingStack(); conflict {
			return fmt.Errorf("Another Bordo stack appears to be running: %s.\n"+
				"Starting a second stack will clash on ports 7401/7402/3000.\n"+
				"Stop the other stack first (e.g. 'cd /opt/bordo/deploy/bordo && docker compose down'), or manage that one instead.\n"+
				"To start anyway, re-run with --force.", desc)
		}
	}
	// Installs from before the console login existed have no password yet.
	pw, created, err := ensureConsolePassword(filepath.Join(platformDir(), ".env"))
	if err != nil {
		return err
	}
	if err := compose("up", "-d", "--remove-orphans").Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	if created {
		fmt.Printf("\nThe web console now requires a login. Username: bordo   Password: %s\n", pw)
		fmt.Println("(stored in " + filepath.Join(platformDir(), ".env") + "; show it again with 'bordo platform password')")
	}
	if !quiet {
		fmt.Println("\nBordo is up. Next:")
		fmt.Println("  bordo platform token    # get the setup token")
		fmt.Println("  bordo setup             # configure (no flags needed on the server)")
	}
	return nil
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
			tok, err := readSetupToken()
			if err != nil {
				return err
			}
			fmt.Println(tok)
			return nil
		},
	}
}

// readSetupToken returns bordod's current setup token from its logs.
func readSetupToken() (string, error) {
	if err := ensureInstalled(); err != nil {
		return "", err
	}
	dir := platformDir()
	out, err := exec.Command("docker", "compose",
		"-f", filepath.Join(dir, "docker-compose.yml"),
		"logs", "bordod").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("reading bordod logs: %w", err)
	}
	tok := extractSetupToken(string(out))
	if tok == "" {
		return "", fmt.Errorf("no setup token found — Bordo may already be configured, or bordod hasn't started yet (check: bordo platform status)")
	}
	return tok, nil
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
	var force bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Pull the latest images and restart services (data preserved)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			return upgradeStack(platformDir(), force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "upgrade even if another Bordo stack is detected (advanced)")
	return cmd
}

// upgradeStack pulls images and restarts the compose stack in dir. When dir is
// the CLI-managed stack but a different Bordo stack already owns the ports (for
// example a git-clone install under /opt/bordo), it refuses: starting a second
// stack would fail on the port bind and leave a half-created project behind.
func upgradeStack(dir string, force bool) error {
	if !force && sameDir(dir, platformDir()) {
		if desc, conflict := conflictingStack(); conflict {
			msg := fmt.Sprintf("Another Bordo stack is running: %s.\n"+
				"Upgrading the CLI-managed stack (%s) would start a second stack and clash on ports 7401/7402/3000.\n", desc, dir)
			const classic = "/opt/bordo/deploy/bordo"
			if _, err := os.Stat(filepath.Join(classic, "docker-compose.yml")); err == nil {
				msg += "Upgrade the running stack instead:\n  bordo upgrade --dir " + classic + "\n"
			}
			msg += "To upgrade this stack anyway, re-run with --force."
			return errors.New(msg)
		}
	}

	syncComposeFile(dir)

	fmt.Println("Pulling latest Bordo images...")
	if err := composeInDir(dir, "pull").Run(); err != nil {
		return fmt.Errorf("docker compose pull: %w", err)
	}

	// The console login arrived after some installs were created: add a password
	// to an existing .env so the console container can start.
	envPath := filepath.Join(dir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		pw, created, err := ensureConsolePassword(envPath)
		if err != nil {
			return err
		}
		if created {
			fmt.Printf("\nThe web console now requires a login. Username: bordo   Password: %s\n", pw)
			fmt.Printf("(stored in %s)\n\n", envPath)
		}
	}

	fmt.Println("Restarting services...")
	if err := composeInDir(dir, "up", "-d", "--remove-orphans").Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	fmt.Println("Upgrade complete.")
	return nil
}

// syncComposeFile brings the compose file up to date with this CLI, because new releases add
// services (the sandbox the agent runs commands in). A CLI-managed install owns its file, so it is
// rewritten when it differs; .env and the data volumes are never touched. A hand-managed file (a
// repo checkout such as /opt/bordo) is left alone, with a note when it lacks the sandbox.
func syncComposeFile(dir string) {
	path := filepath.Join(dir, "docker-compose.yml")
	current, _ := os.ReadFile(path)
	if sameDir(dir, platformDir()) {
		if !bytes.Equal(current, composeYAML) {
			if err := os.WriteFile(path, composeYAML, 0o644); err != nil {
				fmt.Printf("Could not refresh %s: %v\n", path, err)
				return
			}
			fmt.Printf("Updated %s to match this version.\n", path)
		}
		return
	}
	if len(current) > 0 && !strings.Contains(string(current), "bordo-sandbox") {
		fmt.Printf("\nNote: %s has no bordo-sandbox service, so the agent cannot run commands (code_exec).\n"+
			"It is managed by hand: update it from the Bordo repo (e.g. 'git pull' in the checkout), then run 'bordo upgrade' again.\n\n", path)
	}
}

func sameDir(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(aa) == filepath.Clean(bb)
}

// cliProject is the docker compose project name of the CLI-managed stack.
// compose runs with the working directory set to platformDir(), so compose
// derives the project name from that directory's basename (e.g. "deploy").
// Its containers are therefore named "<project>-<service>-N".
func cliProject() string {
	return filepath.Base(platformDir())
}

// conflictingStack reports whether another Bordo stack is already running on
// this host — one that would clash on ports 7401/7402/3000 with the
// CLI-managed stack. It returns a human-readable description (e.g. the
// conflicting container name) and true when a conflict is found.
//
// Heuristic: inspect running containers via `docker ps`; any container whose
// name contains "bordod" but does not belong to the CLI-managed compose
// project is a conflict. As a fallback it also flags the classic git-clone
// install at /opt/bordo/deploy/bordo when a bordod container is running.
//
// If docker is missing or `docker ps` errors, the check is skipped (returns
// "", false) rather than failing — detection is best-effort.
func conflictingStack() (string, bool) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", false
	}
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}\t{{.Ports}}").Output()
	if err != nil {
		return "", false
	}

	prefix := cliProject() + "-"
	altPrefix := cliProject() + "_"
	foundBordod := false
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name := line
		if i := strings.IndexByte(line, '\t'); i >= 0 {
			name = line[:i]
		}
		if !strings.Contains(name, "bordod") {
			continue
		}
		foundBordod = true
		// Skip the CLI-managed stack's own container.
		if strings.HasPrefix(name, prefix) || strings.HasPrefix(name, altPrefix) {
			continue
		}
		return fmt.Sprintf("container %q", name), true
	}

	// Classic git-clone install: a separate compose stack under /opt/bordo.
	if _, err := os.Stat("/opt/bordo/deploy/bordo/docker-compose.yml"); err == nil && foundBordod {
		return "git-clone install at /opt/bordo/deploy/bordo", true
	}

	return "", false
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

// ensureConsolePassword returns BORDO_CONSOLE_PASSWORD from the .env file,
// generating and appending one when it is missing. created reports whether a
// new password was written.
func ensureConsolePassword(envPath string) (pw string, created bool, err error) {
	if pw = readEnvValue(envPath, "BORDO_CONSOLE_PASSWORD"); pw != "" {
		return pw, false, nil
	}
	pw, err = randomToken()
	if err != nil {
		return "", false, err
	}
	existing, _ := os.ReadFile(envPath)
	prefix := ""
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		prefix = "\n"
	}
	f, err := os.OpenFile(envPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return "", false, fmt.Errorf("updating .env: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(prefix + "BORDO_CONSOLE_PASSWORD=" + pw + "\n"); err != nil {
		return "", false, fmt.Errorf("updating .env: %w", err)
	}
	return pw, true, nil
}

func platformPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "password",
		Short: "Show the web console login (username: bordo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureInstalled(); err != nil {
				return err
			}
			pw, _, err := ensureConsolePassword(filepath.Join(platformDir(), ".env"))
			if err != nil {
				return err
			}
			fmt.Printf("Username: bordo\nPassword: %s\n", pw)
			return nil
		},
	}
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

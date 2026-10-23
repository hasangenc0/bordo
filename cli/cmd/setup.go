package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hasangenc0/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// SetupCmd returns the `bordo setup` command.
func SetupCmd() *cobra.Command {
	var (
		server      string
		advanced    bool
		deepSeekKey string
		setupToken  string
		skipGithub  bool
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install, start and configure Bordo on this server, then connect GitHub",
		Long: `One command to get a fresh server running.

Run it on the server itself. It installs and starts the platform if needed,
reads the one-time setup token for you, asks for your DeepSeek API key, then
walks you through connecting GitHub.

Non-interactive: pass --deepseek-key (or set DEEPSEEK_API_KEY) and
--skip-github to run it from a script.

From a different machine, point --server at the bordod URL (e.g. through an
SSH tunnel: ssh -L 7401:localhost:7401 ...), or set it once with
'bordo config set server <url>'. Remote servers are not installed or started
for you, and you must supply the setup token (--setup-token or the prompt).

Power users can run the steps separately: 'bordo platform install',
'bordo platform up', 'bordo platform token', 'bordo github login'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if server == "" {
				cfg, _ := config.Load()
				server = cfg.Server
			}
			if server == "" {
				server = "http://localhost:7401"
			}
			server = strings.TrimRight(server, "/")
			if deepSeekKey == "" {
				deepSeekKey = os.Getenv("DEEPSEEK_API_KEY")
			}
			if setupToken == "" {
				setupToken = os.Getenv("BORDO_SETUP_TOKEN")
			}
			return runSetup(cmd.Context(), setupOpts{
				server:      server,
				advanced:    advanced,
				deepSeekKey: deepSeekKey,
				setupToken:  setupToken,
				skipGithub:  skipGithub,
			})
		},
	}

	cmd.Flags().StringVar(&server, "server", "", "bordod server URL (default: http://localhost:7401)")
	cmd.Flags().BoolVar(&advanced, "advanced", false, "also ask for the public URL, LLM model/endpoint and registry URL")
	cmd.Flags().StringVar(&deepSeekKey, "deepseek-key", "", "DeepSeek API key (or set DEEPSEEK_API_KEY)")
	cmd.Flags().StringVar(&setupToken, "setup-token", "", "one-time setup token (read automatically on the server; or set BORDO_SETUP_TOKEN)")
	cmd.Flags().BoolVar(&skipGithub, "skip-github", false, "don't offer to connect GitHub at the end")
	return cmd
}

type setupOpts struct {
	server      string
	advanced    bool
	deepSeekKey string
	setupToken  string
	skipGithub  bool
}

type setupStatusResp struct {
	Configured bool `json:"configured"`
}

type setupRequest struct {
	SetupToken  string `json:"setup_token"`
	AdminToken  string `json:"admin_token,omitempty"`
	DeepSeekKey string `json:"deepseek_api_key"`
	BaseURL     string `json:"base_url"`
	LLMModel    string `json:"llm_model,omitempty"`
	LLMURL      string `json:"llm_url,omitempty"`
	RegistryURL string `json:"registry_url,omitempty"`
}

type setupResponse struct {
	AdminToken string `json:"admin_token"`
	Message    string `json:"message"`
	Error      string `json:"error"`
}

func runSetup(ctx context.Context, o setupOpts) error {
	server := o.server

	// 1. Make sure bordod is reachable. On this machine, install and start the
	// platform ourselves; for a remote server there is nothing we can start.
	status, err := getSetupStatus(server)
	if err != nil {
		if !isLocalServer(server) {
			return fmt.Errorf("reaching server %s: %w\n\nIs the server running? Check: bordo platform status", server, err)
		}
		if err := ensurePlatformRunning(server); err != nil {
			return err
		}
		status, err = getSetupStatus(server)
		if err != nil {
			return fmt.Errorf("reaching server %s: %w", server, err)
		}
	}
	if status.Configured {
		fmt.Println("This server is already configured.")
		fmt.Println("To update a setting: bordo config set <key> <value>")
		fmt.Println("To connect GitHub:   bordo github login")
		return nil
	}

	fmt.Printf("Setting up Bordo at %s\n\n", server)

	// 2. Collect inputs
	setupToken := o.setupToken
	if setupToken == "" && isLocalServer(server) {
		setupToken = waitForSetupToken(30 * time.Second)
	}
	if setupToken == "" {
		setupToken = prompt("Setup token (run 'bordo platform token'): ", true)
	}
	if setupToken == "" {
		return fmt.Errorf("setup token is required")
	}

	deepSeekKey := o.deepSeekKey
	if deepSeekKey == "" {
		deepSeekKey = prompt("DeepSeek API key (platform.deepseek.com/api_keys): ", true)
	}
	if deepSeekKey == "" {
		return fmt.Errorf("DeepSeek API key is required")
	}

	baseURL, llmModel, llmURL, registryURL := server, "", "", ""
	if o.advanced {
		if v := prompt(fmt.Sprintf("Public URL of this server [%s]: ", server), false); v != "" {
			baseURL = v
		}
		fmt.Println("\nOptional settings (press Enter to skip):")
		llmModel = prompt("  LLM model override [deepseek-chat]: ", false)
		llmURL = prompt("  LLM endpoint URL (leave empty for DeepSeek): ", false)
		registryURL = prompt("  Container registry URL: ", false)
	}

	// 3. Send setup request
	req := setupRequest{
		SetupToken:  setupToken,
		DeepSeekKey: deepSeekKey,
		BaseURL:     baseURL,
		LLMModel:    llmModel,
		LLMURL:      llmURL,
		RegistryURL: registryURL,
	}
	resp, err := postSetup(server, req)
	if err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("setup rejected: %s", resp.Error)
	}

	// 4. Auto-save to CLI config
	cfg, _ := config.Load()
	cfg.Server = server
	cfg.Token = resp.AdminToken
	if err := config.Save(cfg); err != nil {
		fmt.Printf("\nWarning: could not save CLI config: %v\n", err)
	}

	consoleURL := "http://localhost:3000"
	if u, err := url.Parse(server); err == nil && u.Hostname() != "" {
		consoleURL = "http://" + u.Hostname() + ":3000"
	}

	fmt.Printf(`
╔════════════════════════════════════════════════════════╗
║  Bordo setup complete!                                 ║
╚════════════════════════════════════════════════════════╝

  Admin token saved to CLI config. You're ready to go.
  Console:  %s   (change the port if CONSOLE_PORT differs)
  Console login:  bordo platform password

`, consoleURL)

	if !o.skipGithub {
		connectGithubAfterSetup(ctx)
	}

	fmt.Print(`
  Try:
    bordo agent chat "create a new java web service called hello-api"

  To update settings later:
    bordo config set deepseek_api_key <new-key>
    bordo config set registry_url ghcr.io/your-org

`)
	return nil
}

// connectGithubAfterSetup offers the GitHub device flow as the last setup step.
// Failures only print a hint: setup itself already succeeded.
func connectGithubAfterSetup(ctx context.Context) {
	if !isTerminal(os.Stdin) {
		fmt.Println("  Next: connect GitHub with 'bordo github login'.")
		return
	}
	ans := strings.ToLower(prompt("Connect GitHub now so Bordo can build your code? [Y/n]: ", false))
	if ans == "n" || ans == "no" {
		fmt.Println("  Later: bordo github login")
		return
	}
	if err := runGithubLogin(ctx, ""); err != nil {
		fmt.Printf("\nCould not connect GitHub: %v\n  Retry later with: bordo github login\n", err)
	}
}

// ensurePlatformRunning installs the platform if it isn't yet and starts it,
// then waits for bordod to answer on server.
func ensurePlatformRunning(server string) error {
	if err := ensureInstalled(); err != nil {
		fmt.Println("Installing the Bordo platform...")
		if err := runPlatformInstall(false, true); err != nil {
			return err
		}
	}
	fmt.Println("Starting Bordo...")
	if err := runPlatformUp(false, true); err != nil {
		return err
	}
	fmt.Print("Waiting for bordod")
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := getSetupStatus(server); err == nil {
			fmt.Println(" ready.")
			return nil
		}
		fmt.Print(".")
		time.Sleep(2 * time.Second)
	}
	fmt.Println()
	return fmt.Errorf("bordod did not come up within 90s — check: bordo platform status / bordo platform logs bordod")
}

// waitForSetupToken polls bordod's logs for the setup token; it may not be
// logged for a moment after the container starts. Returns "" on timeout.
func waitForSetupToken(timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for {
		if tok, err := readSetupToken(); err == nil {
			return tok
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(2 * time.Second)
	}
}

// isLocalServer reports whether server points at this machine, where the CLI
// can manage the platform and read the setup token itself.
func isLocalServer(server string) bool {
	u, err := url.Parse(server)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// serverSettingKeys are settings stored on the Bordo server (encrypted settings
// store), as opposed to local CLI config keys. 'bordo config set' routes these
// to the server.
var serverSettingKeys = map[string]bool{
	"deepseek_api_key": true,
	"base_url":         true,
	"llm_model":        true,
	"llm_url":          true,
	"registry_url":     true,
}

// setServerSetting updates a single setting on the Bordo server via /v1/settings.
func setServerSetting(key, value string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Server == "" {
		return fmt.Errorf("not logged in — run: bordo login --server <url> --token <token>")
	}

	body, _ := json.Marshal(map[string]string{"key": key, "value": value})
	req, err := http.NewRequest(http.MethodPost, cfg.Server+"/v1/settings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error: %s", result["error"])
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func getSetupStatus(server string) (*setupStatusResp, error) {
	resp, err := http.Get(server + "/setup/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var s setupStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func postSetup(server string, req setupRequest) (*setupResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(server+"/setup", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var s setupResponse
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("unexpected response: %s", string(raw))
	}
	return &s, nil
}

func prompt(label string, required bool) string {
	fmt.Print(label)
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)
	if required && input == "" {
		fmt.Print("  (required) " + label)
		fmt.Scanln(&input)
		input = strings.TrimSpace(input)
	}
	return input
}

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/hasangenc0/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// SetupCmd returns the `bordo setup` command.
func SetupCmd() *cobra.Command {
	var server string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Complete first-run setup for a Bordo server",
		Long: `Interactive wizard that configures a freshly installed Bordo server.

Run it on the server itself — it defaults to http://localhost:7401, so no
flags are needed:

  bordo platform token      # grab the one-time setup token
  bordo setup               # configure (prompts for the token + DeepSeek key)

From a different machine, point --server at the bordod URL (e.g. through an
SSH tunnel: ssh -L 7401:localhost:7401 ...), or set it once with
'bordo config set server <url>'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if server == "" {
				cfg, _ := config.Load()
				server = cfg.Server
			}
			if server == "" {
				server = "http://localhost:7401"
			}
			server = strings.TrimRight(server, "/")
			return runSetup(server)
		},
	}

	cmd.Flags().StringVar(&server, "server", "", "bordod server URL (default: http://localhost:7401)")
	return cmd
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

func runSetup(server string) error {
	// 1. Check status
	status, err := getSetupStatus(server)
	if err != nil {
		return fmt.Errorf("reaching server %s: %w\n\nIs the server running? Check: bordo platform status", server, err)
	}
	if status.Configured {
		fmt.Println("This server is already configured.")
		fmt.Println("To update a setting: bordo config set <key> <value>")
		return nil
	}

	fmt.Printf("Setting up Bordo at %s\n\n", server)

	// 2. Collect inputs
	setupToken := prompt("Setup token (run 'bordo platform token'): ", true)
	if setupToken == "" {
		return fmt.Errorf("setup token is required")
	}

	deepSeekKey := prompt("DeepSeek API key (platform.deepseek.com/api_keys): ", true)
	if deepSeekKey == "" {
		return fmt.Errorf("DeepSeek API key is required")
	}

	baseURL := prompt(fmt.Sprintf("Public URL of this server [%s]: ", server), false)
	if baseURL == "" {
		baseURL = server
	}

	fmt.Println("\nOptional settings (press Enter to skip):")
	llmModel := prompt("  LLM model override [deepseek-chat]: ", false)
	llmURL := prompt("  LLM endpoint URL (leave empty for DeepSeek): ", false)
	registryURL := prompt("  Container registry URL: ", false)

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

  Try:
    bordo project list
    bordo agent chat "create a new java web service called hello-api"

  Next, connect GitHub to enable builds & deploys:
    bordo github setup
    bordo github install

  Console:  %s   (change the port if CONSOLE_PORT differs)

  To update settings later:
    bordo config set deepseek_api_key <new-key>
    bordo config set registry_url ghcr.io/your-org

`, consoleURL)

	return nil
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

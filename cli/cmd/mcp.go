package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bordo-io/bordo/cli/internal/config"
	"github.com/spf13/cobra"
)

// McpCmd returns the `bordo mcp` command tree.
func McpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP (Model Context Protocol) integrations",
	}
	cmd.AddCommand(mcpSetupCmd())
	return cmd
}

func mcpSetupCmd() *cobra.Command {
	var (
		agent  string
		server string
		token  string
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure an AI agent to use Bordo as an MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			serverURL := cfg.Server
			if server != "" {
				serverURL = server
			}
			tok := cfg.Token
			if token != "" {
				tok = token
			}

			mcpURL := serverURL + "/api/mcp"

			switch agent {
			case "claude-code":
				return setupClaudeCode(mcpURL, tok)
			case "gemini":
				return setupGemini(mcpURL, tok)
			case "print":
				return printMCPConfig(mcpURL, tok)
			default:
				return fmt.Errorf("unknown agent %q; valid values: claude-code, gemini, print", agent)
			}
		},
	}

	cmd.Flags().StringVar(&agent, "agent", "", "target agent: claude-code | gemini | print (required)")
	cmd.Flags().StringVar(&server, "server", "", "bordo server URL (default: from config)")
	cmd.Flags().StringVar(&token, "token", "", "bordo token (default: from config)")
	_ = cmd.MarkFlagRequired("agent")

	return cmd
}

// setupClaudeCode writes/merges ~/.claude/mcp.json with the Bordo MCP entry.
func setupClaudeCode(mcpURL, token string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home directory: %w", err)
	}
	mcpPath := filepath.Join(home, ".claude", "mcp.json")

	// Load existing file if present.
	existing := map[string]any{}
	if data, err := os.ReadFile(mcpPath); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parsing %s: %w", mcpPath, err)
		}
	}

	servers, _ := existing["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	updated := servers["bordo"] != nil
	servers["bordo"] = map[string]any{
		"type": "http",
		"url":  mcpURL,
		"headers": map[string]string{
			"Authorization": "Bearer " + token,
		},
	}
	existing["mcpServers"] = servers

	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o700); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(mcpPath, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", mcpPath, err)
	}

	if updated {
		fmt.Printf("✓ Updated existing Bordo MCP config at %s\n", mcpPath)
	} else {
		fmt.Printf("✓ Bordo MCP server added to Claude Code config at %s\n", mcpPath)
	}
	return nil
}

// setupGemini writes/merges ~/.gemini/settings.json with the Bordo MCP entry.
func setupGemini(mcpURL, token string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home directory: %w", err)
	}
	settingsPath := filepath.Join(home, ".gemini", "settings.json")

	existing := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parsing %s: %w", settingsPath, err)
		}
	}

	// Merge: replace any existing "bordo" entry, keep others.
	var servers []any
	if s, ok := existing["mcpServers"].([]any); ok {
		for _, entry := range s {
			if m, ok := entry.(map[string]any); ok {
				if m["name"] == "bordo" {
					continue
				}
			}
			servers = append(servers, entry)
		}
	}
	servers = append(servers, map[string]any{
		"name":    "bordo",
		"httpUrl": mcpURL,
		"headers": map[string]string{
			"Authorization": "Bearer " + token,
		},
	})
	existing["mcpServers"] = servers

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		return fmt.Errorf("creating .gemini directory: %w", err)
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", settingsPath, err)
	}

	fmt.Printf("✓ Bordo MCP server added to Gemini config at %s\n", settingsPath)
	return nil
}

// printMCPConfig prints the MCP config JSON to stdout.
func printMCPConfig(mcpURL, token string) error {
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"bordo": map[string]any{
				"type": "http",
				"url":  mcpURL,
				"headers": map[string]string{
					"Authorization": "Bearer " + token,
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

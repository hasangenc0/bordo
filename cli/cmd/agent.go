package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hasangenc0/bordo/cli/internal/config"
	"github.com/spf13/cobra"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// AgentCmd returns the agent command group.
func AgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Interact with the Bordo AI agent",
	}
	cmd.AddCommand(agentChatCmd())
	return cmd
}

type wsFrame struct {
	Type     string         `json:"type"`
	Role     string         `json:"role,omitempty"`
	Content  string         `json:"content,omitempty"`
	ToolName string         `json:"tool_name,omitempty"`
	Input    map[string]any `json:"input,omitempty"`
}

func agentChatCmd() *cobra.Command {
	var sessionID string

	cmd := &cobra.Command{
		Use:   "chat <message>",
		Short: "Send a message to the agent and stream the response",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			agentURL := cfg.AgentURL
			if agentURL == "" {
				agentURL = "http://localhost:7402"
			}

			// The agent authenticates with the internal agent token, not the
			// admin token. Prefer agent_token; fall back to token for setups
			// where they coincide.
			authToken := cfg.AgentToken
			if authToken == "" {
				authToken = cfg.Token
			}

			// Create a new chat session if not reusing one.
			if sessionID == "" {
				sessionID, err = createChatSession(agentURL, authToken)
				if err != nil {
					return fmt.Errorf("creating chat session: %w", err)
				}
				fmt.Printf("[session: %s]\n", sessionID[:8])
			}

			message := strings.Join(args, " ")

			// Convert http:// → ws:// for the WebSocket URL.
			wsBase := strings.Replace(agentURL, "http://", "ws://", 1)
			wsBase = strings.Replace(wsBase, "https://", "wss://", 1)
			wsURL := wsBase + "/ws/chat?chat_id=" + sessionID
			// Pass token as query param — WebSocket API in browsers can't set headers.
			if authToken != "" {
				wsURL += "&token=" + authToken
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				return fmt.Errorf("connecting to agent: %w", err)
			}
			defer conn.CloseNow()

			// Send the user message in a goroutine so that if the server sends a "history"
			// frame first, neither side deadlocks waiting for the other to read.
			sendErr := make(chan error, 1)
			go func() {
				out := map[string]string{"type": "message", "role": "user", "content": message}
				sendErr <- wsjson.Write(ctx, conn, out)
			}()

			// Stream frames until "done" or "error".
			// "history" frames (sent for sessions with prior messages) are silently discarded.
			for {
				var f wsFrame
				if err := wsjson.Read(ctx, conn, &f); err != nil {
					return fmt.Errorf("reading response: %w", err)
				}
				switch f.Type {
				case "history":
					// prior chat history loaded on reconnect — ignore
				case "message":
					fmt.Print(f.Content)
				case "tool_call":
					input, _ := json.Marshal(f.Input)
					fmt.Printf("\n[tool: %s] %s\n", f.ToolName, string(input))
				case "done":
					fmt.Println()
					if werr := <-sendErr; werr != nil {
						return fmt.Errorf("sending message: %w", werr)
					}
					return conn.Close(websocket.StatusNormalClosure, "")
				case "error":
					fmt.Println()
					return fmt.Errorf("agent error: %s", f.Content)
				}
			}
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "reuse an existing chat session ID")
	return cmd
}

// createChatSession POSTs to /chats and returns the new session ID.
func createChatSession(agentURL, token string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, agentURL+"/chats", strings.NewReader(`{"title":""}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		msg := strings.TrimSpace(string(body))
		if resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("agent rejected the request (401): set the agent token with 'bordo config set agent_token <BORDO_INTERNAL_TOKEN>' (from your deploy .env)")
		}
		return "", fmt.Errorf("agent returned %s: %s", resp.Status, msg)
	}

	var r struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.ID == "" {
		return "", fmt.Errorf("agent returned empty session ID")
	}
	return r.ID, nil
}

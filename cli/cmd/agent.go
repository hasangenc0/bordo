package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
	var interactive bool

	cmd := &cobra.Command{
		Use:   "chat [message]",
		Short: "Chat with the agent — one-shot with a message, or interactive with none",
		Long: `Send a message to the agent and stream the reply.

With a message argument it runs one-shot and exits. With no message (or
--interactive) it opens an interactive session — type messages, see replies,
and keep context across turns. Exit with 'exit', 'quit', or Ctrl-D.`,
		Args: cobra.ArbitraryArgs,
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
			// admin token. Prefer agent_token; fall back to token.
			authToken := cfg.AgentToken
			if authToken == "" {
				authToken = cfg.Token
			}

			if sessionID == "" {
				sessionID, err = createChatSession(agentURL, authToken)
				if err != nil {
					return fmt.Errorf("creating chat session: %w", err)
				}
			}

			wsBase := strings.Replace(agentURL, "http://", "ws://", 1)
			wsBase = strings.Replace(wsBase, "https://", "wss://", 1)
			wsURL := wsBase + "/ws/chat?chat_id=" + sessionID
			if authToken != "" {
				wsURL += "&token=" + authToken
			}

			// Cancel cleanly on Ctrl-C.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				return fmt.Errorf("connecting to agent: %w", err)
			}
			defer conn.CloseNow()

			runInteractive := interactive || len(args) == 0

			// One-shot: send the message, stream one turn, done.
			if !runInteractive {
				if err := sendChatMessage(ctx, conn, strings.Join(args, " ")); err != nil {
					return fmt.Errorf("sending message: %w", err)
				}
				if err := streamChatTurn(ctx, conn); err != nil {
					return err
				}
				return conn.Close(websocket.StatusNormalClosure, "")
			}

			// Interactive REPL.
			fmt.Printf("Bordo agent — session %s. Type 'exit' or Ctrl-D to quit.\n", sessionID[:8])

			// If a message was also passed with -i, send it as the first turn.
			if len(args) > 0 {
				msg := strings.Join(args, " ")
				fmt.Printf("› %s\n", msg)
				if err := sendChatMessage(ctx, conn, msg); err != nil {
					return fmt.Errorf("sending message: %w", err)
				}
				if err := streamChatTurn(ctx, conn); err != nil && ctx.Err() == nil {
					return err
				}
			}

			scanner := bufio.NewScanner(os.Stdin)
			for ctx.Err() == nil {
				fmt.Print("› ")
				if !scanner.Scan() {
					break // EOF / Ctrl-D
				}
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				if line == "exit" || line == "quit" {
					break
				}
				if err := sendChatMessage(ctx, conn, line); err != nil {
					if ctx.Err() != nil {
						break
					}
					return fmt.Errorf("sending message: %w", err)
				}
				if err := streamChatTurn(ctx, conn); err != nil {
					if ctx.Err() != nil {
						break
					}
					return err
				}
			}
			fmt.Println()
			return conn.Close(websocket.StatusNormalClosure, "")
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "reuse an existing chat session ID")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "interactive session (default when no message is given)")
	return cmd
}

// sendChatMessage writes a user message frame to the agent WebSocket.
func sendChatMessage(ctx context.Context, conn *websocket.Conn, content string) error {
	return wsjson.Write(ctx, conn, map[string]string{
		"type": "message", "role": "user", "content": content,
	})
}

// streamChatTurn reads and prints frames until the agent signals "done" (end of
// one reply) or "error". "history" frames are ignored.
func streamChatTurn(ctx context.Context, conn *websocket.Conn) error {
	for {
		var f wsFrame
		if err := wsjson.Read(ctx, conn, &f); err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
		switch f.Type {
		case "history":
			// prior chat history on reconnect — ignore
		case "message":
			fmt.Print(f.Content)
		case "tool_call":
			input, _ := json.Marshal(f.Input)
			fmt.Printf("\n[tool: %s] %s\n", f.ToolName, string(input))
		case "done":
			fmt.Println()
			return nil
		case "error":
			fmt.Println()
			return fmt.Errorf("agent error: %s", f.Content)
		}
	}
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

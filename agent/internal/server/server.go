// Package server implements the bordo-agent HTTP server.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/bordo-io/bordo/agent/internal/tools"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
)

// Config holds agent server configuration.
type Config struct {
	Port     int
	APIKey   string
	LogLevel string
}

// Server is the bordo-agent HTTP server.
type Server struct {
	cfg       Config
	logger    *slog.Logger
	router    *chi.Mux
	http      *http.Server
	sessions  *SessionStore
	tools     *tools.ToolRegistry
	anthropic *AnthropicClient
	upgrader  websocket.Upgrader
}

// Run creates and starts the server, blocking until ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Port == 0 {
		cfg.Port = 7402
	}

	cpURL := os.Getenv("BORDO_CP_URL")
	if cpURL == "" {
		cpURL = "http://localhost:7401"
	}
	cpToken := os.Getenv("BORDO_TOKEN")
	cpClient := tools.NewCPClient(cpURL, cpToken)

	s := &Server{
		cfg:       cfg,
		logger:    newLogger(cfg.LogLevel),
		router:    chi.NewRouter(),
		sessions:  NewSessionStore(),
		tools:     tools.NewBordoToolRegistry(cpClient),
		anthropic: NewAnthropicClient(cfg.APIKey),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.Recoverer)

	s.router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.router.Get("/ws/chat", s.handleChat)
	s.router.Get("/sessions", s.handleListSessions)
	s.router.Post("/mcp", s.handleMCP)
	s.router.Get("/mcp/tools", s.handleListTools)
	s.router.Post("/mcp/tools/{name}", s.handleCallTool)

	s.http = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.http.Addr, err)
	}
	s.logger.Info("bordo-agent listening", "addr", s.http.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("bordo-agent shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.http.Shutdown(shutCtx)
	}
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	sessionID := middleware.GetReqID(r.Context())
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Warn("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	session := s.sessions.Create(sessionID)
	s.logger.Info("chat session started", "session_id", sessionID)
	defer s.logger.Info("chat session ended", "session_id", sessionID)

	sendFrame := func(f WSFrame) error {
		b, _ := json.Marshal(f)
		return conn.WriteMessage(websocket.TextMessage, b)
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req ChatMessage
		if err := json.Unmarshal(msg, &req); err != nil {
			_ = sendFrame(WSFrame{Type: "error", Content: "invalid message format"})
			continue
		}

		userContent := req.Content
		session.AddMessage("user", userContent)

		if s.cfg.APIKey == "" && s.anthropic.Token == "" {
			reply := fmt.Sprintf("(no model provider configured — set CF_API_TOKEN; received: %q)", userContent)
			session.AddMessage("assistant", reply)
			if err := sendFrame(WSFrame{Type: "message", Role: "assistant", Content: reply}); err != nil {
				break
			}
			_ = sendFrame(WSFrame{Type: "done"})
			continue
		}

		// Build Anthropic tools list from the registry.
		allTools := s.tools.List()
		anthropicTools := make([]AnthropicTool, 0, len(allTools))
		for _, t := range allTools {
			schema := t.Schema
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			anthropicTools = append(anthropicTools, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
			})
		}

		// History up to (not including) the message we just added.
		history := session.ToAnthropicMessages()
		// The last entry is the user message we just added; use the full history.

		var fullReply string
		_, loopErr := s.anthropic.RunAgentLoop(
			r.Context(),
			history,
			anthropicTools,
			func(ctx context.Context, name string, input map[string]any) (any, error) {
				tool := s.tools.Get(name)
				if tool == nil {
					return nil, fmt.Errorf("unknown tool: %s", name)
				}
				return tool.Call(ctx, input)
			},
			func(text string) {
				_ = sendFrame(WSFrame{Type: "message", Role: "assistant", Content: text})
				fullReply += text
			},
			func(name string, input map[string]any) {
				_ = sendFrame(WSFrame{Type: "tool_call", ToolName: name, Input: input})
			},
		)

		if loopErr != nil {
			s.logger.Error("agent loop error", "error", loopErr)
			_ = sendFrame(WSFrame{Type: "error", Content: "model error: " + loopErr.Error()})
			_ = sendFrame(WSFrame{Type: "done"})
			continue
		}

		session.AddMessage("assistant", fullReply)
		if err := sendFrame(WSFrame{Type: "done"}); err != nil {
			break
		}
	}
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": s.sessions.List(),
	})
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, MCPResponse{Error: "invalid request"})
		return
	}

	tool := s.tools.Get(req.Tool)
	if tool == nil {
		writeJSON(w, http.StatusNotFound, MCPResponse{Error: "tool not found: " + req.Tool})
		return
	}

	result, err := tool.Call(r.Context(), req.Params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, MCPResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, MCPResponse{Result: result})
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	type toolInfo struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Schema      map[string]any `json:"schema,omitempty"`
	}
	all := s.tools.List()
	out := make([]toolInfo, 0, len(all))
	for _, t := range all {
		out = append(out, toolInfo{Name: t.Name, Description: t.Description, Schema: t.Schema})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": out})
}

func (s *Server) handleCallTool(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	tool := s.tools.Get(name)
	if tool == nil {
		writeJSON(w, http.StatusNotFound, MCPResponse{Error: "tool not found: " + name})
		return
	}
	var params map[string]any
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		params = map[string]any{}
	}
	result, err := tool.Call(r.Context(), params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, MCPResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, MCPResponse{Result: result})
}


func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeWSError(conn *websocket.Conn, msg string) {
	b, _ := json.Marshal(map[string]string{"error": msg})
	_ = conn.WriteMessage(websocket.TextMessage, b)
}

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
	"path/filepath"
	"time"

	"github.com/bordo-io/bordo/agent/internal/chatstore"
	"github.com/bordo-io/bordo/agent/internal/tools"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
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
	chats     *chatstore.Store
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

	// Fetch LLM config from bordod if not already set via env.
	// This lets the admin configure the AI key once via the setup wizard
	// rather than needing it in docker-compose env vars.
	if cfg.APIKey == "" {
		if rc, err := fetchRuntimeConfig(cpURL, cpToken); err == nil {
			if cfg.APIKey == "" {
				cfg.APIKey = rc.DeepSeekAPIKey
			}
			if os.Getenv("BORDO_LLM_MODEL") == "" && rc.LLMModel != "" {
				os.Setenv("BORDO_LLM_MODEL", rc.LLMModel)
			}
			if os.Getenv("BORDO_LLM_URL") == "" && rc.LLMURL != "" {
				os.Setenv("BORDO_LLM_URL", rc.LLMURL)
			}
		}
	}

	chatDBPath := os.Getenv("BORDO_CHAT_DB")
	if chatDBPath == "" {
		home, _ := os.UserHomeDir()
		chatDBPath = filepath.Join(home, ".bordo", "agent-chats.db")
	}
	chatDB, err := chatstore.Open(chatDBPath)
	if err != nil {
		return fmt.Errorf("open chat store: %w", err)
	}

	s := &Server{
		cfg:       cfg,
		logger:    newLogger(cfg.LogLevel),
		router:    chi.NewRouter(),
		sessions:  NewSessionStore(),
		tools:     tools.NewBordoToolRegistry(cpClient),
		anthropic: NewAnthropicClient(cfg.APIKey),
		chats:     chatDB,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.Recoverer)

	agentToken := os.Getenv("BORDO_AGENT_TOKEN")
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if agentToken == "" || r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			// Accept token via Authorization header or ?token= query param
			// (query param lets the browser WebSocket connect without custom headers).
			tok := r.Header.Get("Authorization")
			if tok == "Bearer "+agentToken || r.URL.Query().Get("token") == agentToken {
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		})
	})

	s.router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.router.Get("/ws/chat", s.handleChat)
	s.router.Get("/sessions", s.handleListSessions)
	s.router.Post("/mcp", s.handleMCP)
	s.router.Get("/mcp/tools", s.handleListTools)
	s.router.Post("/mcp/tools/{name}", s.handleCallTool)

	// Chat persistence routes
	s.router.Get("/chats", s.handleListChats)
	s.router.Post("/chats", s.handleCreateChat)
	s.router.Get("/chats/{id}", s.handleGetChat)
	s.router.Delete("/chats/{id}", s.handleDeleteChat)
	s.router.Put("/chats/{id}/title", s.handleUpdateChatTitle)

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
	chatID := r.URL.Query().Get("chat_id")
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Warn("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	session := s.sessions.Create(sessionID)
	s.logger.Info("chat session started", "session_id", sessionID, "chat_id", chatID)
	defer s.logger.Info("chat session ended", "session_id", sessionID)

	sendFrame := func(f WSFrame) error {
		b, _ := json.Marshal(f)
		return conn.WriteMessage(websocket.TextMessage, b)
	}

	// Load history from chatstore and pre-populate session
	if chatID != "" && s.chats != nil {
		msgs, _ := s.chats.GetMessages(r.Context(), chatID)
		if len(msgs) > 0 {
			_ = sendFrame(WSFrame{Type: "history", Messages: msgs})
			for _, m := range msgs {
				session.AddMessage(m.Role, m.Content)
			}
		}
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

		// Persist user message
		if chatID != "" && s.chats != nil {
			_ = s.chats.AppendMessage(r.Context(), chatID, "user", userContent)
			// Auto-title from first message
			existing, _ := s.chats.GetMessages(r.Context(), chatID)
			if len(existing) == 1 {
				title := userContent
				if len([]rune(title)) > 40 {
					title = string([]rune(title)[:40]) + "…"
				}
				_ = s.chats.UpdateTitle(r.Context(), chatID, title)
			}
		}

		if s.anthropic.Token == "" {
			reply := fmt.Sprintf("(no model provider configured — set DEEPSEEK_API_KEY; received: %q)", userContent)
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
		if chatID != "" && s.chats != nil && fullReply != "" {
			_ = s.chats.AppendMessage(r.Context(), chatID, "assistant", fullReply)
		}
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

func (s *Server) handleListChats(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.chats.ListSessions(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if sessions == nil {
		sessions = []chatstore.ChatSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"chats": sessions})
}

func (s *Server) handleCreateChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	id := uuid.New().String()
	if err := s.chats.CreateSession(r.Context(), id, req.Title); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	sess, _ := s.chats.GetSession(r.Context(), id)
	writeJSON(w, http.StatusCreated, sess)
}

func (s *Server) handleGetChat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := s.chats.GetSession(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if sess == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	msgs, _ := s.chats.GetMessages(r.Context(), id)
	if msgs == nil {
		msgs = []chatstore.ChatMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": sess, "messages": msgs})
}

func (s *Server) handleDeleteChat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.chats.DeleteSession(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleUpdateChatTitle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}
	if err := s.chats.UpdateTitle(r.Context(), id, req.Title); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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


type runtimeConfig struct {
	DeepSeekAPIKey string `json:"deepseek_api_key"`
	LLMModel       string `json:"llm_model"`
	LLMURL         string `json:"llm_url"`
}

func fetchRuntimeConfig(cpURL, token string) (*runtimeConfig, error) {
	req, err := http.NewRequest(http.MethodGet, cpURL+"/v1/runtime-config", nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runtime-config: status %d", resp.StatusCode)
	}
	var rc runtimeConfig
	if err := json.NewDecoder(resp.Body).Decode(&rc); err != nil {
		return nil, err
	}
	return &rc, nil
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

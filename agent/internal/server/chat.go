package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// handleChat serves one WebSocket client. The connection only carries frames: the work a message
// starts runs as a background run (see runs.go) that outlives the connection, so closing the UI
// or the mobile app does not interrupt it.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	sessionID := middleware.GetReqID(r.Context())
	chatID := r.URL.Query().Get("chat_id")
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Warn("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// One run per chat. Without a chat id nothing persists, so the run is tied to this connection.
	runKey := chatID
	if runKey == "" {
		runKey = "session:" + sessionID
	}

	session := s.sessions.Create(sessionID)
	s.logger.Info("chat session started", "session_id", sessionID, "chat_id", chatID)
	defer s.logger.Info("chat session ended", "session_id", sessionID)

	var writeMu sync.Mutex // gorilla/websocket allows one writer at a time
	write := func(b []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(websocket.TextMessage, b)
	}
	sendFrame := func(f WSFrame) error {
		b, _ := json.Marshal(f)
		return write(b)
	}
	sub := &subscriber{send: write}
	defer s.runs.detachEverywhere(sub)
	if chatID == "" {
		defer s.runs.stop(runKey) // nobody can come back to this run
	}

	// A turn may be in flight from an earlier connection. Look before loading history so the
	// replay and the stored messages do not both show its final reply.
	active := s.runs.get(runKey)
	replay := active != nil && !active.isPersisted()

	if chatID != "" && s.chats != nil {
		msgs, _ := s.chats.GetMessages(r.Context(), chatID)
		if len(msgs) > 0 {
			_ = sendFrame(WSFrame{Type: "history", Messages: msgs})
			for _, m := range msgs {
				session.AddMessage(m.Role, m.Content)
			}
		}
	}
	if replay {
		_ = sendFrame(WSFrame{Type: "running"})
		active.attach(sub)
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

		if req.Type == "stop" {
			if !s.runs.stop(runKey) {
				_ = sendFrame(WSFrame{Type: "done"}) // nothing was running; just clear the client's busy state
			}
			continue
		}

		r2, ctx, ok := s.runs.begin(runKey)
		if !ok {
			_ = sendFrame(WSFrame{Type: "error", Content: "The agent is still working on the previous message in this chat. Wait for it, or press stop."})
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

		// The stored history is authoritative: another connection may have run the previous turn,
		// and this connection's in-memory session would not have its reply.
		history := session.ToAnthropicMessages()
		if chatID != "" && s.chats != nil {
			if msgs, err := s.chats.GetMessages(r.Context(), chatID); err == nil {
				history = history[:0]
				for _, m := range msgs {
					if m.Role == "user" || m.Role == "assistant" {
						history = append(history, AnthropicMessage{Role: m.Role, Content: m.Content})
					}
				}
			}
		}

		r2.attach(sub)
		go s.runTurn(ctx, r2, chatID, session, history, userContent)
	}
}

// runTurn runs one agent turn to completion, independent of any connection. Everything the user
// should see goes through run.emit, which both streams it to attached clients and keeps it for
// clients that connect later.
func (s *Server) runTurn(ctx context.Context, r *run, chatID string, session *Session, history []AnthropicMessage, userContent string) {
	defer s.runs.end(r)

	persist := func(reply string) {
		if reply == "" {
			return
		}
		session.AddMessage("assistant", reply)
		if chatID != "" && s.chats != nil {
			pctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.chats.AppendMessage(pctx, chatID, "assistant", reply)
		}
	}
	finish := func() {
		r.mu.Lock()
		r.persisted = true
		r.mu.Unlock()
		r.emit(WSFrame{Type: "done"})
	}

	// Pick up a key configured after startup (e.g. via 'bordo setup').
	s.refreshModelProvider()

	if s.anthropic.Token == "" {
		reply := fmt.Sprintf("(no model provider configured — run 'bordo setup' to set the DeepSeek key; received: %q)", userContent)
		r.emit(WSFrame{Type: "message", Role: "assistant", Content: reply})
		persist(reply)
		finish()
		return
	}

	// Build the tools list from the registry.
	allTools := s.tools.List()
	anthropicTools := make([]AnthropicTool, 0, len(allTools))
	for _, t := range allTools {
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		anthropicTools = append(anthropicTools, AnthropicTool{Name: t.Name, Description: t.Description, InputSchema: schema})
	}

	var fullReply string
	_, loopErr := s.anthropic.RunAgentLoop(
		ctx,
		history,
		anthropicTools,
		func(ctx context.Context, name string, input map[string]any) (any, error) {
			tool := s.tools.Get(name)
			if tool == nil {
				return nil, fmt.Errorf("unknown tool: %s", name)
			}
			if tool.ApprovalSummary != nil {
				if aerr := s.requestApproval(ctx, name, actionKindFromTool(name), tool.ApprovalSummary(input), input, func(ev ActionEvent) { r.emit(ev) }); aerr != nil {
					r.emit(WSFrame{Type: "tool_result", ToolName: name, Error: aerr.Error()})
					return nil, aerr
				}
			}
			result, err := tool.Call(ctx, input)
			trf := WSFrame{Type: "tool_result", ToolName: name}
			if err != nil {
				trf.Error = err.Error()
			} else if raw, ok := result.(json.RawMessage); ok {
				var decoded any
				if json.Unmarshal(raw, &decoded) == nil {
					trf.Data = decoded
				} else {
					trf.Data = string(raw)
				}
			} else {
				trf.Data = result
			}
			r.emit(trf)
			if tool.ApprovalSummary != nil {
				return result, err // its approval card already showed the outcome
			}
			// Emit action event after tool execution.
			r.emit(ActionEvent{
				Type:         "action",
				ActionID:     uuid.New().String(),
				ActionKind:   actionKindFromTool(name),
				ActionTarget: actionTargetFromInput(input),
				ActionStatus: "auto_approved",
				ActionTs:     time.Now().UnixMilli(),
			})
			return result, err
		},
		func(text string) {
			r.emit(WSFrame{Type: "message", Role: "assistant", Content: text})
			if fullReply != "" {
				fullReply += "\n\n" // keep text from separate model turns apart in saved history
			}
			fullReply += text
		},
		func(name string, input map[string]any) {
			r.emit(WSFrame{Type: "tool_call", ToolName: name, Input: input})
		},
	)

	switch {
	case ctx.Err() != nil:
		// Stopped by the user, cut short by a restart, or out of time. Say which, and keep the
		// work so far in the history so "continue" has something to continue from.
		r.mu.Lock()
		stopped, shutdown := r.stopped, r.shutdown
		r.mu.Unlock()
		note := "(stopped)"
		switch {
		case shutdown:
			note = "(interrupted: the agent restarted; say \"continue\" to pick this up again)"
		case !stopped:
			note = fmt.Sprintf("(this turn ran out of time after %s; say \"continue\" to carry on)", turnTimeout)
		}
		r.emit(WSFrame{Type: "message", Role: "assistant", Content: note})
		if fullReply != "" {
			fullReply += "\n\n"
		}
		persist(fullReply + note)
	case loopErr != nil:
		s.logger.Error("agent loop error", "error", loopErr)
		r.emit(WSFrame{Type: "error", Content: "model error: " + loopErr.Error()})
		if fullReply != "" {
			persist(fullReply + "\n\n(interrupted by a model error; the user may ask to continue)")
		}
	default:
		persist(fullReply)
	}
	finish()
}

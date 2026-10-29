package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hasangenc0/bordo/agent/internal/chatstore"
	"github.com/hasangenc0/bordo/agent/internal/tools"
)

// chatHarness is a real chat server (WebSocket handler, chat store, run manager) whose model is
// scripted and whose single tool blocks until released, so a turn can be held mid-flight.
type chatHarness struct {
	t       *testing.T
	srv     *httptest.Server
	chats   *chatstore.Store
	release chan struct{} // closing it lets the slow tool finish
	started chan struct{} // closed when the slow tool is running
	once    sync.Once
}

func newChatHarness(t *testing.T) *chatHarness {
	t.Helper()
	h := &chatHarness{t: t, release: make(chan struct{}), started: make(chan struct{})}

	chats, err := chatstore.Open(filepath.Join(t.TempDir(), "chats.db"))
	if err != nil {
		t.Fatal(err)
	}
	h.chats = chats

	reg := tools.NewToolRegistry()
	reg.Register(&tools.Tool{
		Name: "t", Description: "blocks until released", Schema: map[string]any{"type": "object", "properties": map[string]any{}},
		CallFn: func(ctx context.Context, _ map[string]any) (any, error) {
			h.once.Do(func() { close(h.started) })
			select {
			case <-h.release:
				return "done", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	// Model: first call asks for the slow tool, the next answers.
	calls := 0
	var mu sync.Mutex
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			_, _ = w.Write([]byte(toolReply()))
			return
		}
		_, _ = w.Write([]byte(textReply("All done.", "stop")))
	}))
	t.Cleanup(llm.Close)

	s := &Server{
		logger:          newLogger("error"),
		sessions:        NewSessionStore(),
		tools:           reg,
		anthropic:       &AnthropicClient{Token: "t", Model: "m", APIURL: llm.URL, http: llm.Client()},
		chats:           chats,
		runs:            newRuns(),
		providerFromEnv: true,
		upgrader:        websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
	}
	h.srv = httptest.NewServer(http.HandlerFunc(s.handleChat))
	t.Cleanup(h.srv.Close)
	t.Cleanup(func() { s.runs.stopAll() })

	if err := chats.CreateSession(context.Background(), "chat1", "t"); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *chatHarness) dial() *websocket.Conn {
	h.t.Helper()
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.srv.URL, "http")+"?chat_id=chat1", nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { c.Close() })
	return c
}

// frames reads frames until one of type stop arrives (or the deadline), returning every frame.
func frames(t *testing.T, c *websocket.Conn, until string) []map[string]any {
	t.Helper()
	var out []map[string]any
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, b, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %q, got %v after %v", until, err, out)
		}
		var f map[string]any
		_ = json.Unmarshal(b, &f)
		out = append(out, f)
		if f["type"] == until {
			return out
		}
	}
}

func hasFrame(fs []map[string]any, typ, contains string) bool {
	for _, f := range fs {
		if f["type"] == typ && strings.Contains(strings.ToLower(toStr(f["content"])+toStr(f["tool_name"])), strings.ToLower(contains)) {
			return true
		}
	}
	return false
}

func toStr(v any) string { s, _ := v.(string); return s }

func (h *chatHarness) messages() []chatstore.ChatMessage {
	msgs, err := h.chats.GetMessages(context.Background(), "chat1")
	if err != nil {
		h.t.Fatal(err)
	}
	return msgs
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func send(t *testing.T, c *websocket.Conn, v map[string]any) {
	t.Helper()
	if err := c.WriteJSON(v); err != nil {
		t.Fatal(err)
	}
}

func TestTurnSurvivesTheClientClosing(t *testing.T) {
	h := newChatHarness(t)
	c := h.dial()
	send(t, c, map[string]any{"type": "message", "content": "deploy it"})
	<-h.started
	c.Close() // the user closes the UI while the agent is mid-task

	close(h.release)
	waitFor(t, "the reply to be saved", func() bool {
		msgs := h.messages()
		return len(msgs) == 2 && msgs[1].Role == "assistant" && strings.Contains(msgs[1].Content, "All done.")
	})
}

func TestReconnectReplaysAndFollowsARunningTurn(t *testing.T) {
	h := newChatHarness(t)
	c1 := h.dial()
	send(t, c1, map[string]any{"type": "message", "content": "deploy it"})
	frames(t, c1, "tool_call")
	c1.Close()

	c2 := h.dial() // the user comes back while the task is still running
	close(h.release)
	got := frames(t, c2, "done")
	if !hasFrame(got, "history", "") {
		t.Errorf("missing history: %v", got)
	}
	if !hasFrame(got, "running", "") {
		t.Errorf("the client must be told a turn is running: %v", got)
	}
	if !hasFrame(got, "tool_call", "t") {
		t.Errorf("what happened before reconnecting must be replayed: %v", got)
	}
	if !hasFrame(got, "message", "all done") {
		t.Errorf("the live reply must follow: %v", got)
	}
	// The reply is stored once, not once per connection.
	waitFor(t, "a single stored reply", func() bool { return len(h.messages()) == 2 })
}

func TestStopCancelsARunAndKeepsTheWork(t *testing.T) {
	h := newChatHarness(t)
	c := h.dial()
	send(t, c, map[string]any{"type": "message", "content": "deploy it"})
	<-h.started
	send(t, c, map[string]any{"type": "stop"})
	got := frames(t, c, "done")
	if !hasFrame(got, "message", "(stopped)") {
		t.Fatalf("the user must be told it stopped: %v", got)
	}
	waitFor(t, "the stop to be recorded", func() bool {
		msgs := h.messages()
		return len(msgs) == 2 && strings.Contains(msgs[1].Content, "(stopped)")
	})

	// And the chat accepts a new message afterwards.
	send(t, c, map[string]any{"type": "message", "content": "again"})
	if got := frames(t, c, "done"); hasFrame(got, "error", "") {
		t.Fatalf("a stopped chat must accept new work: %v", got)
	}
}

func TestSecondMessageWhileRunningIsRejected(t *testing.T) {
	h := newChatHarness(t)
	c := h.dial()
	send(t, c, map[string]any{"type": "message", "content": "deploy it"})
	<-h.started
	send(t, c, map[string]any{"type": "message", "content": "and also this"})
	got := frames(t, c, "error")
	if !hasFrame(got, "error", "still working") {
		t.Fatalf("got %v", got)
	}
	close(h.release)
	frames(t, c, "done")
	if n := len(h.messages()); n != 2 {
		t.Fatalf("the rejected message must not be stored, have %d messages", n)
	}
}

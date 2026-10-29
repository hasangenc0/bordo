package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Regression: an assistant message that only carries tool calls must still serialize "content",
// or providers answer 422 "messages[N]: missing field `content`".
func TestOAIMessageAlwaysHasContent(t *testing.T) {
	msgs := []oaiMessage{
		{Role: "assistant", ToolCalls: []oaiToolCall{{ID: "1", Type: "function"}}},
		{Role: "tool", ToolCallID: "1"},
	}
	for _, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"content":""`) {
			t.Errorf("%s message missing content: %s", m.Role, b)
		}
	}
	b, _ := json.Marshal(oaiMessage{Role: "user", Content: "hi"})
	if strings.Contains(string(b), "tool_calls") || strings.Contains(string(b), "tool_call_id") {
		t.Errorf("empty tool fields should stay omitted: %s", b)
	}
}

// fakeLLM serves scripted chat-completion replies, one per call.
func fakeLLM(t *testing.T, replies []string, calls *int) *AnthropicClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := *calls
		*calls++
		if i >= len(replies) {
			i = len(replies) - 1
		}
		if strings.HasPrefix(replies[i], "HTTP ") {
			code, _ := strconv.Atoi(strings.Fields(replies[i])[1])
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":"scripted"}`))
			return
		}
		_, _ = w.Write([]byte(replies[i]))
	}))
	t.Cleanup(srv.Close)
	old := llmRetryDelay
	llmRetryDelay = time.Millisecond
	t.Cleanup(func() { llmRetryDelay = old })
	return &AnthropicClient{Token: "t", Model: "m", APIURL: srv.URL, http: srv.Client()}
}

func textReply(text, finish string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
		"message": map[string]any{"role": "assistant", "content": text}, "finish_reason": finish}}})
	return string(b)
}

func toolReply() string {
	return `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"t","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
}

func runLoop(c *AnthropicClient) (string, error) {
	return c.RunAgentLoop(context.Background(), []AnthropicMessage{{Role: "user", Content: "go"}}, nil,
		func(context.Context, string, map[string]any) (any, error) { return "ok", nil }, nil, nil)
}

func TestLoopContinuesAfterAnAnnouncedStep(t *testing.T) {
	var calls int
	c := fakeLLM(t, []string{
		textReply("The build is queued. Let me check its status.", "stop"),
		textReply("The build succeeded.", "stop"),
	}, &calls)
	out, err := runLoop(c)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(out, "The build succeeded.") {
		t.Fatalf("an announced-but-untaken step must be followed up: calls=%d out=%q", calls, out)
	}
}

func TestLoopContinuesAfterTheTokenLimit(t *testing.T) {
	var calls int
	c := fakeLLM(t, []string{textReply("part one ", "length"), textReply("and the rest.", "stop")}, &calls)
	out, err := runLoop(c)
	if err != nil || calls != 2 || out != "part one and the rest." {
		t.Fatalf("calls=%d out=%q err=%v", calls, out, err)
	}
}

func TestLoopStopsWhenTheAnswerIsFinal(t *testing.T) {
	for _, final := range []string{
		"Done. The app is serving.",
		"Which region should I deploy to?",
		"I deployed it; I'll wait for your go-ahead on the next one?",
	} {
		var calls int
		c := fakeLLM(t, []string{textReply(final, "stop")}, &calls)
		if _, err := runLoop(c); err != nil || calls != 1 {
			t.Errorf("%q: a finished answer must not be nudged (calls=%d err=%v)", final, calls, err)
		}
	}
}

func TestLoopNudgeIsBounded(t *testing.T) {
	var calls int
	c := fakeLLM(t, []string{textReply("Let me look.", "stop")}, &calls)
	if _, err := runLoop(c); err != nil {
		t.Fatal(err)
	}
	if calls != maxNudges+1 {
		t.Fatalf("a model that keeps announcing must be nudged %d times then left alone, got %d calls", maxNudges, calls)
	}
}

func TestLoopSaysWhenItHitsTheStepLimit(t *testing.T) {
	var calls int
	c := fakeLLM(t, []string{toolReply()}, &calls)
	out, err := runLoop(c)
	if err != nil || calls != maxIterations || !strings.Contains(out, "Say \"continue\"") {
		t.Fatalf("calls=%d err=%v out=%q", calls, err, out)
	}
}

func TestLoopRetriesTransientModelErrors(t *testing.T) {
	var calls int
	c := fakeLLM(t, []string{"HTTP 503", "HTTP 429", textReply("Done.", "stop")}, &calls)
	out, err := runLoop(c)
	if err != nil || calls != 3 || out != "Done." {
		t.Fatalf("calls=%d out=%q err=%v", calls, out, err)
	}
	calls = 0
	c = fakeLLM(t, []string{"HTTP 401"}, &calls)
	if _, err := runLoop(c); err == nil || calls != 1 {
		t.Fatalf("a rejected key must not be retried (calls=%d err=%v)", calls, err)
	}
}

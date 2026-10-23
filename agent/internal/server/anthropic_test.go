package server

import (
	"encoding/json"
	"strings"
	"testing"
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

package server

// ChatMessage is an inbound WebSocket message from the client.
type ChatMessage struct {
	Type    string `json:"type"`    // "message"
	Role    string `json:"role"`
	Content string `json:"content"`
	Token   string `json:"token,omitempty"`
}

// WSFrame is an outbound WebSocket frame from the agent.
type WSFrame struct {
	Type     string         `json:"type"`                // "message" | "tool_call" | "done" | "error" | "history"
	Role     string         `json:"role,omitempty"`      // "assistant" (type=message)
	Content  string         `json:"content,omitempty"`   // type=message or type=error
	ToolName string         `json:"tool_name,omitempty"` // type=tool_call
	Input    map[string]any `json:"input,omitempty"`     // type=tool_call
	Messages any            `json:"messages,omitempty"`  // type=history
}

// MCPRequest is a tool call posted to /mcp.
type MCPRequest struct {
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params"`
}

// MCPResponse is the result of an MCP tool call.
type MCPResponse struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

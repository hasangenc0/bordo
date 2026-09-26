package server

// ChatMessage is sent/received over the WebSocket chat endpoint.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

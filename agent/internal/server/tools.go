package server

import (
	"context"
	"sync"
)

// Tool is an MCP tool the agent can call.
type Tool struct {
	Name        string
	Description string
	callFn      func(ctx context.Context, params map[string]any) (any, error)
}

func (t *Tool) Call(ctx context.Context, params map[string]any) (any, error) {
	return t.callFn(ctx, params)
}

// ToolRegistry holds all registered MCP tools.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]*Tool)}
}

func (r *ToolRegistry) Register(t *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

func (r *ToolRegistry) Get(name string) *Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// PingTool is the built-in proof-of-life tool.
func PingTool() *Tool {
	return &Tool{
		Name:        "ping",
		Description: "Returns pong — proves the MCP loop works",
		callFn: func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]bool{"pong": true}, nil
		},
	}
}

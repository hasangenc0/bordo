// Package tools defines the MCP Tool type, ToolRegistry, and all Bordo tools.
package tools

import (
	"context"
	"sync"
)

// Tool is an MCP tool the agent can invoke.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	CallFn      func(ctx context.Context, params map[string]any) (any, error)
}

func (t *Tool) Call(ctx context.Context, params map[string]any) (any, error) {
	return t.CallFn(ctx, params)
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

func (r *ToolRegistry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// PingTool is the built-in proof-of-life tool.
func PingTool() *Tool {
	return &Tool{
		Name:        "ping",
		Description: "Returns pong — proves the MCP loop works",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		CallFn: func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]bool{"pong": true}, nil
		},
	}
}

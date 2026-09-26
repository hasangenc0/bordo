# agent/

The Bordo AI agent — a WebSocket chat endpoint + MCP tool server.

## What it will do (EP-07)

- Host a WebSocket chat endpoint (`/ws/chat`) for the console
- Host an MCP (Model Context Protocol) tool server (`/mcp`) for any compatible model
- Persist chat sessions
- Route tool calls to the Bordo control-plane gRPC API
- Support pluggable model providers (default: Claude via Anthropic API)

## Planned issues

- **BRD-040** — Agent chat loop + MCP tool registry
- **BRD-041** — MCP tools for build/release/observe ops

## Implementation direction

```
agent/
  main.go              entry point
  internal/
    mcp/               MCP tool server + tool registry
    chat/              WebSocket chat endpoint, session store
    provider/          model provider abstraction (Claude, OpenAI, etc.)
    client/            gRPC client to bordod
```

See [ADR-0005](../docs/architecture/ADR-0005-agent-first-ux-and-mcp.md) for rationale.

## Dependencies

Depends on EP-01 (control-plane API) being available. Start with BRD-040.

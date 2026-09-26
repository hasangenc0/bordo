# ADR-0005: Agent-first UX with MCP Tools

**Status:** Accepted  
**Date:** 2026-09-26  
**Deciders:** Project founders

## Context

Traditional IDPs have dashboards, forms, and tables for every operation. This is
powerful but imposes a steep learning curve and context switching cost. The goal for
Bordo is that a solo builder can drive the entire platform through natural language
without learning a UI.

We need to decide how the AI agent connects to the platform and what protocol to use
for the tool interface.

## Decision

1. The AI agent is a first-class component (not a wrapper bolted on later). It runs as
   a separate process alongside `bordod` but talks to it via the same gRPC API that
   the CLI uses.

2. The agent exposes platform operations as **MCP (Model Context Protocol) tools**.
   MCP is an open standard for connecting language models to tools and resources.
   This means:
   - Any MCP-compatible model (Claude, GPT-4, Gemini, local Llama) can drive Bordo.
   - The tool list is the authoritative description of what the agent can do.
   - The console's sidebar panels are rendered from structured tool results, not from
     a separate data-fetching layer.

3. The default model provider is **Anthropic Claude** (via the Anthropic API). The
   provider is configurable in `bordod.yaml`.

4. The web console communicates with the agent over **WebSocket** for streaming chat
   responses and over **REST** for session management.

## Tool surface (initial)

| Tool | Description |
|---|---|
| `project_create` | Scaffold and register a new project |
| `project_list` | List projects and their status |
| `build_trigger` | Trigger a build for a project |
| `build_status` | Get build status and logs |
| `deploy` | Deploy a version to one or more regions |
| `deploy_status` | Get deployment status and rollout progress |
| `rollback` | Roll back to a previous version |
| `metrics_query` | Query metrics for a project or region |
| `logs_query` | Query logs |
| `region_list` | List registered regions and cluster health |
| `region_add` | Bootstrap k3s on a VM and register a region |

## Alternatives considered

- **Custom REST API only** (no agent/MCP) — leaves the UX work to the user.
- **LangChain / LangGraph** — adds a heavy framework dependency; MCP is more minimal
  and model-agnostic.
- **Function calling per-model** — every model has a different protocol; MCP normalizes
  this to one interface.
- **Dashboard-first with chat as a secondary surface** — inverts the priority; our
  thesis is that chat-first reduces the "IDP learning curve" barrier.

## Consequences

- The MCP tool surface is the primary API contract — it must be stable and
  well-documented.
- Changes to control-plane capabilities must be reflected in MCP tools.
- The agent process adds a dependency on an external LLM provider (API key required);
  a fully offline mode (local model) is a future goal.
- Rich structured responses from tools enable the console to render cards, charts,
  and tables from chat messages rather than maintaining separate fetch logic.

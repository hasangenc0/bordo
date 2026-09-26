# console/

The Bordo web console — a chat-first React + TypeScript + Node BFF application.

## What it will be (EP-08)

A web application where users interact with the Bordo platform through natural
language chat. The AI agent handles requests; the console renders structured results
as rich cards. There are no pre-built dashboards.

## Layout (planned)

```
console/
  app/                React + TypeScript (Vite) SPA
    src/
      components/
        Chat.tsx       Main chat pane
        Sidebar.tsx    Navigation: Projects/Build/Release/Observe/Metrics
        cards/         Structured result cards (ProjectCard, BuildCard, DeployCard, ...)
      hooks/
        useChat.ts     WebSocket chat connection
      App.tsx
      main.tsx
    index.html
    vite.config.ts
    tsconfig.json
    package.json
  bff/                Node.js BFF (Express or Fastify)
    src/
      server.ts       Express/Fastify server
      routes/         API routes proxied to bordod + agent
    package.json
    tsconfig.json
  package.json        Workspace root (npm workspaces)
```

## Planned issues

- **BRD-050** — Web console shell: chat + sidebar

## Dependencies

Depends on EP-07 (agent WebSocket endpoint + MCP tools).

## Design note

The sidebar sections (Projects, Build, Release, Observe, Metrics) don't navigate to
separate pages — they pre-load a chat context. Everything is a conversation.

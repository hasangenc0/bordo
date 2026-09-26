# Bordo Console

The Bordo web console — a chat-first interface with a sidebar for Projects, Build, Release, Observe, and Metrics.

## Architecture

- **frontend/** — Vite + React + TypeScript SPA (port 5173)
- **bff/** — Express + TypeScript BFF that proxies to the Bordo agent (port 3001)

## Quick start

```bash
# From this directory
npm install
npm run dev
```

Open http://localhost:5173 in your browser.

## Configuration

1. Start the Bordo agent: `./bin/bordod serve` (port 7401) and the agent (port 7402)
2. Open the console at http://localhost:5173
3. Go to **Settings** in the sidebar and enter your bearer token
4. The chat pane connects to `ws://localhost:7402/ws/chat`

## Environment variables (frontend)

| Variable | Default | Description |
|---|---|---|
| `VITE_AGENT_WS_URL` | `ws://localhost:7402/ws/chat` | Agent WebSocket URL |
| `VITE_BFF_URL` | `/api` | BFF base path (proxied by Vite) |

## Environment variables (bff)

| Variable | Default | Description |
|---|---|---|
| `BORDO_AGENT_URL` | `http://localhost:7402` | Agent HTTP base URL |
| `PORT` | `3001` | BFF listen port |

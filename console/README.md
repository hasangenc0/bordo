# Bordo Console

The web UI for Bordo. Chat with your Bordo agent, see your apps, deploy them to regions, watch metrics and logs, and ask the agent to change code.

## What's in it

| Page | What you can do |
|---|---|
| **Chat** (`/`) | Talk to the agent. Tool calls show as cards: code diffs, deploy status, the command to join a region. History is kept per conversation. |
| **Apps** (`/apps`) | List apps, create one from a template, deploy to one or more regions. |
| **App detail** (`/apps/:name`) | Metrics, logs, deployments (with rollback), builds, variables, and a **Code** tab that hands a change request to the agent. |
| **Regions** (`/regions`) | See each region's nodes and the apps running in it. Add a region and get the one-time join command. |
| **Observe** (`/observe`) | Run your own PromQL / LogQL across all regions. |

On every page except Chat, the agent is available in a side panel.

### Code changes

Asking the agent to change an app's code makes it clone the repo, edit on its own `bordo/...` branch, show you the diff, push the branch and optionally open a pull request. It never pushes to the default branch and cannot edit `.github/workflows`. The agent needs GitHub connected (`bordo github login`) and `git` in its image (the agent Dockerfile installs it).

### Metrics

Charts query VictoriaMetrics through the control plane (`/v1/observe/metrics`). If no observability backend is configured the pages say so instead of showing empty charts. App charts use the OTLP `job` label (the app's `service.name`) and conventional HTTP/runtime metric names. If your apps export different names, use the Observe page or change the presets in `frontend/src/lib/metrics.ts`.

## Architecture

- **frontend/**: Vite + React + TypeScript SPA (port 5173 in dev).
- **bff/**: Express dev proxy (port 3001). Forwards `/api/*` to the agent and relays the chat WebSocket. In production, nginx (`deploy/bordo/nginx-console.conf.template`) does this job and injects the agent token.

All control-plane calls go `browser → /api/v1/* → agent → control plane`. The console has no login of its own: anyone who can reach it can use the agent, including pushing code and deploying. Keep it on a private network or behind your own auth.

## Develop

```bash
npm install
npm run dev          # BFF + Vite
```

Open http://localhost:5173. You need `bordod` (:7401) and the agent (:7402) running; the BFF reads `BORDO_AGENT_URL` (default `http://localhost:7402`) and `BORDO_AGENT_TOKEN`.

| Variable | Where | Default | Description |
|---|---|---|---|
| `BORDO_AGENT_URL` | bff | `http://localhost:7402` | Agent base URL |
| `BORDO_AGENT_TOKEN` | bff | none | Bearer token for the agent |
| `PORT` | bff | `3001` | BFF listen port |
| `VITE_AGENT_WS_URL` | frontend | same host, `/ws/chat` | Override the chat WebSocket URL |

Note: `npm run build` (`tsc -b`) also emits `frontend/vite.config.js`, which Vite prefers over `vite.config.ts`. It is git-ignored and regenerated on every build, so rerun `npx tsc -b` after editing the `.ts` config.

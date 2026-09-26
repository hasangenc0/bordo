# Template: ts-react-app

**Language**: TypeScript / React  
**Runtime**: SPA (Single Page Application)  
**Issue**: BRD-070

## What this template produces

A React + TypeScript SPA with:
- Vite build tooling
- React Router for client-side navigation
- Pre-configured Bordo auth token (from env: `VITE_BORDO_TOKEN`)
- OpenTelemetry browser tracing
- Multi-stage Dockerfile (Node build → nginx serve)
- nginx health endpoint (`GET /health` → 200)
- ESLint + Prettier configured

## Variables

| Variable | Description | Default |
|---|---|---|
| `ProjectName` | App name (kebab-case) | required |

## Status

🚧 Template files not yet created — see **BRD-070** (TS/Node app framework epic).

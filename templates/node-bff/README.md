# Template: node-bff

**Language**: TypeScript / Node.js  
**Runtime**: Backend For Frontend (BFF)  
**Issue**: BRD-070

## What this template produces

A Node.js BFF with:
- Express or Fastify (configurable)
- Structured JSON logging (pino)
- OpenTelemetry Node.js auto-instrumentation
- Example proxy route to a Java backend
- Health endpoint (`GET /health` → 200)
- Dockerfile (multi-stage: Node build → slim runtime)
- TypeScript strict mode, ESLint + Prettier

## Variables

| Variable | Description | Default |
|---|---|---|
| `ProjectName` | App name (kebab-case) | required |
| `BackendUrl` | URL of the backend service to proxy | `http://localhost:8080` |

## Status

🚧 Template files not yet created — see **BRD-070** (TS/Node app framework epic).

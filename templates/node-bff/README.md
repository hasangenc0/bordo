# node-bff

Node.js + Fastify + TypeScript Backend-for-Frontend golden-path template for Bordo.

## Scaffold

```bash
bordo project create --template node-bff --name my-bff --var BackendURL=http://my-service:8080
cd my-bff
cp .env.example .env
```

## Run locally

```bash
npm install
npm run dev      # tsx watch on http://localhost:3000
```

## Environment variables

| Variable | Description | Default |
|---|---|---|
| `PORT` | Server port | `3000` |
| `BACKEND_URL` | Java backend base URL to proxy | `http://backend:8080` |
| `OTEL_SERVICE_NAME` | Service name for traces | project name |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry OTLP HTTP endpoint | (disabled if unset) |

## Routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check — returns `{status: "ok"}` |
| `*` | `/api/v1/*` | Proxied to `BACKEND_URL/api/v1/*` |

## Build

```bash
npm run build    # compiles TypeScript to dist/
npm start        # runs dist/index.js
```

## Container

```bash
docker build -t my-bff .
docker run -p 3000:3000 -e BACKEND_URL=http://backend:8080 my-bff
# GET http://localhost:3000/health → {"status":"ok","service":"my-bff"}
```

## Bordo integration

- Health check: `GET /health` (port 3000)
- Structured JSON logging via pino (built into Fastify)
- OTel auto-instrumentation enabled when `OTEL_EXPORTER_OTLP_ENDPOINT` is set
- Proxy all `/api/v1/*` calls to the Java backend — pairs with `java-web-service` template

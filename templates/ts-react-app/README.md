# ts-react-app

Vite + React + TypeScript SPA golden-path template for Bordo.

## Scaffold

```bash
bordo project create --template ts-react-app --name my-frontend
cd my-frontend
cp .env.example .env
```

## Run locally

```bash
npm install
npm run dev      # Vite dev server on http://localhost:5173
```

## Environment variables

| Variable | Description | Default |
|---|---|---|
| `VITE_API_URL` | Backend API base URL | `http://localhost:8080` |
| `VITE_OTEL_ENDPOINT` | OpenTelemetry OTLP HTTP endpoint | (disabled if unset) |
| `VITE_OTEL_SERVICE_NAME` | Service name for traces | `app` |
| `VITE_APP_VERSION` | App version injected at build time | `0.1.0` |

## Build

```bash
npm run build    # outputs to dist/
```

## Container

```bash
docker build -t my-frontend .
docker run -p 8080:80 my-frontend
# GET http://localhost:8080/health → 200 ok
```

## Bordo integration

- Health check: `GET /health` (nginx, port 80)
- OTel traces sent to `VITE_OTEL_ENDPOINT/v1/traces` if set
- Backend calls go to `VITE_API_URL` — wire to your Java web service or Node BFF

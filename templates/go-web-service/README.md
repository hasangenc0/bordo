# {{.ProjectName}}

A Go HTTP web service scaffolded by Bordo. It uses only the standard library, so
the container image is a static binary on distroless (~10MB) and the running
process holds a few MB of RAM instead of the hundreds a JVM needs.

## Endpoints

| Method | Path | Returns |
|---|---|---|
| GET | `/api/v1/hello` | `{"message": "hello from {{.ProjectName}}"}` |
| GET | `/healthz` | `{"status": "UP"}` |
| GET | `/actuator/health` | `{"status": "UP"}` (matches Bordo's readiness probe) |

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | Port the server listens on (Bordo sets this) |

## Run it locally

```bash
go run .
curl localhost:8080/api/v1/hello
```

## Deploy

Push to `main` and GitHub Actions builds the image to GHCR, then:

```bash
bordo deploy create <project-id> ghcr.io/<owner>/{{.ProjectName}}:<tag> \
  --region <region> --expose nodeport
```

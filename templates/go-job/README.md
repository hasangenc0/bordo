# {{.ProjectName}}

A Go scheduled job scaffolded by Bordo. It runs `runJob` on a fixed interval,
shuts down cleanly on SIGTERM, and uses only the standard library, so the image
is a static binary on distroless and the process holds a few MB of RAM.

The job runs as a long-lived process (one Deployment) with an internal ticker,
not a Kubernetes CronJob. Put your work in `runJob` in `main.go`.

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `BORDO_JOB_INTERVAL` | `60s` | How often the job runs (`30s`, `5m`, `1h`, ...) |
| `PORT` | `8080` | Port for the health endpoint (Bordo sets this) |

## Run it locally

```bash
BORDO_JOB_INTERVAL=5s go run .
```

## Deploy

Push to `main` and GitHub Actions builds the image to GHCR, then:

```bash
bordo deploy create <project-id> ghcr.io/<owner>/{{.ProjectName}}:<tag> --region <region>
```

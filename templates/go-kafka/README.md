# {{.ProjectName}}

A Go Kafka service scaffolded by Bordo. It runs a consumer and a producer side by
side on [segmentio/kafka-go](https://github.com/segmentio/kafka-go) (pure Go, no
CGO), so the image is a static binary on distroless and the process holds a few MB
of RAM instead of the hundreds a JVM-based consumer needs.

- The **consumer** reads the topic and logs each message (put your logic in `consume`).
- The **producer** sends a heartbeat every 10s as a working example (replace `produce`).

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `KAFKA_BOOTSTRAP_SERVERS` | `localhost:9092` | Comma-separated broker list |
| `KAFKA_TOPIC` | `events` | Topic to consume and produce |
| `KAFKA_GROUP_ID` | `{{.ProjectName}}` | Consumer group id |
| `PORT` | `8080` | Port for the health endpoint (Bordo sets this) |

## Run it locally

With a broker on `localhost:9092`:

```bash
go run .
```

## Deploy

Push to `main` and GitHub Actions builds the image to GHCR, then:

```bash
bordo deploy create <project-id> ghcr.io/<owner>/{{.ProjectName}}:<tag> \
  --region <region>
```

Set your broker address with `KAFKA_BOOTSTRAP_SERVERS` in the release environment.

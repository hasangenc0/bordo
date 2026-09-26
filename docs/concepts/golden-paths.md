# Golden Paths

A **golden path** is an opinionated, working starting point for a specific kind of
application. It encodes best practices — project structure, configuration, build
pipeline, Dockerfile, health checks, metrics, logging, CI config — so you don't
have to rediscover them.

## Available templates

| Template | Language | What it produces |
|---|---|---|
| `java-web-service` | Java / Spring Boot | HTTP API with OpenAPI, structured logging, health endpoints, OTel |
| `java-worker` | Java | Background processor, queue integration, retry/DLQ, OTel |
| `java-kafka` | Java | Kafka consumer + producer with schema registry support |
| `java-job` | Java | Scheduled / cron job with distributed locking |
| `java-data-layer` | Java | JDBC data-access layer, connection pool, Flyway migrations, any RDBMS |
| `ts-react-app` | TypeScript / React | Vite SPA, pre-wired for Bordo auth + metrics |
| `node-bff` | TypeScript / Node | Express/Fastify BFF, proxies to backend services |

## Using a template

```bash
# Via CLI
bordo project create my-api --template java-web-service

# Via chat
> create a new Java web service called my-api
```

What Bordo does:
1. Copies the template directory tree and expands variables (`ProjectName`, `GroupId`, etc.)
2. Registers the project in the control-plane catalog
3. Creates the initial GitOps desired-state entry
4. Optionally triggers an initial build

## Template contract

Every template directory contains:

```
templates/<name>/
  README.md          # what this template produces
  template.yaml      # Bordo template manifest
  <source files>     # with {{.Var}} placeholders
```

`template.yaml` schema (excerpt):
```yaml
name: java-web-service
description: Java Spring Boot HTTP API
language: java
runtime: web-service
variables:
  - name: ProjectName
    prompt: "Project name (e.g. my-api)"
    default: ""
  - name: GroupId
    prompt: "Maven group ID (e.g. com.example)"
    default: "com.example"
build:
  dockerfile: Dockerfile
  context: .
```

See `api/openapi/` for the full template manifest schema.

## Contributing a template

Read `docs/guides/contributing-a-template.md`.

# java-worker

Golden-path template for a Bordo worker service built on `bordo-worker`.

## What you get

- `BordoWorker<T>` base class — retry logic (3 attempts, exponential backoff), DLQ routing, OTel span per message
- Spring Boot Actuator health endpoint at `/actuator/health`
- Prometheus metrics at `/actuator/prometheus`
- OTel tracing via `opentelemetry-spring-boot-starter`

## Scaffold

```bash
bordo project create --template java-worker \
  --var ProjectName=invoice-processor \
  --var GroupId=com.acme \
  --var QueueName=invoices
```

## Run locally

```bash
./mvnw spring-boot:run
```

## Build image

```bash
bordo build --project invoice-processor
```

## Template variables

| Variable      | Description                  | Default       |
|---------------|------------------------------|---------------|
| `ProjectName` | App name (kebab-case)        | `my-worker`   |
| `GroupId`     | Java package / Maven groupId | `com.example` |
| `QueueName`   | Queue this worker consumes   | `tasks`       |

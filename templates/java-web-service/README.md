# Template: java-web-service

**Language**: Java 21 (Spring Boot 3.x)  
**Runtime**: HTTP API  
**Issue**: BRD-006

## What this template produces

A Spring Boot HTTP API with:
- REST endpoints with OpenAPI documentation
- Actuator health/readiness endpoints (`/actuator/health`, `/actuator/readyz`)
- Structured JSON logging
- OpenTelemetry auto-instrumentation (metrics + traces)
- Multi-stage Dockerfile (JDK build → JRE runtime)
- Maven wrapper (`mvnw`)

## Variables

| Variable | Description | Default |
|---|---|---|
| `ProjectName` | Project name (kebab-case) | required |
| `GroupId` | Maven group ID | `com.example` |

## Usage

```bash
bordo project create my-api --template java-web-service
# or locally:
bordo template expand java-web-service \
  --var ProjectName=my-api \
  --var GroupId=com.acme \
  --output ./my-api
```

## After scaffolding

```bash
cd my-api
./mvnw package -DskipTests
java -jar target/*.jar
curl http://localhost:8080/actuator/health  # {"status":"UP"}
curl http://localhost:8080/api/v1/hello     # {"message":"hello"}
```

## Status

🚧 Template files not yet created — see **BRD-006**.

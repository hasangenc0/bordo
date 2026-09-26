# sdk/java/

Java SDK — opinionated runtime libraries for apps managed by Bordo.

## Modules (EP-09)

| Module | Description | Issue |
|---|---|---|
| `bordo-worker` | Background worker base class with retry, DLQ, OTel | BRD-060 |
| `bordo-kafka` | Kafka consumer + producer abstractions, schema registry | BRD-060 |
| `bordo-job` | Scheduled job with distributed locking | BRD-060 |
| `bordo-data` | JDBC data layer, HikariCP, Flyway migrations, any RDBMS | BRD-061 |

## Maven coordinates (planned)

```xml
<dependency>
  <groupId>io.bordo</groupId>
  <artifactId>bordo-worker</artifactId>
  <version>0.1.0</version>
</dependency>
```

## Status

🚧 SDK modules not yet created — see EP-09 issues (BRD-060, BRD-061).

The `java-web-service` template (BRD-006) uses Spring Boot directly, not this SDK.
Worker, Kafka, and job runtimes are built on top of the SDK.

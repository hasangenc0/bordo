# {{.ProjectName}} — Java data-layer template

Spring Boot service using `bordo-data` for database access: HikariCP connection pool, Flyway schema migrations, and OTel JDBC tracing out of the box.

## Features

- **Any RDBMS**: PostgreSQL, MySQL, SQLite, or any JDBC-compatible database
- **Connection pooling**: HikariCP with configurable pool size
- **Migrations**: Flyway runs `db/migration/V*.sql` on startup automatically
- **Tracing**: Every SQL query appears as a span in your OTel backend
- **Type-safe CRUD**: `JdbcRepository<T, ID>` base class with upsert, list, delete, count

## Scaffolding

```bash
bordo project create --name {{.ProjectName}} --template java-data-layer
```

## Configuration

| Env variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `jdbc:postgresql://localhost:5432/{{.ProjectName}}` | JDBC connection string |
| `DB_USER` | `postgres` | Database username |
| `DB_PASSWORD` | _(empty)_ | Database password |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4318` | OTel collector |

## Local development

```bash
# Start PostgreSQL
docker run -d --name pg -e POSTGRES_DB={{.ProjectName}} -e POSTGRES_PASSWORD=secret -p 5432:5432 postgres:16

# Run the service
DATABASE_URL=jdbc:postgresql://localhost:5432/{{.ProjectName}} \
DB_USER=postgres DB_PASSWORD=secret \
./mvnw spring-boot:run
```

## Adding a new entity

1. Create an entity class in `entity/`
2. Write `db/migration/V<n>__create_<table>.sql`
3. Extend `JdbcRepository<MyEntity, UUID>` in `repository/`
4. Wire the repository into your service

## Build

```bash
./mvnw package -DskipTests
docker build -t {{.ProjectName}}:latest .
```

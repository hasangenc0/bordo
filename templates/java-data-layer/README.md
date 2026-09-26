# Template: java-data-layer

**Language**: Java 21  
**Runtime**: Standalone library (used by other Java templates)  
**Issue**: BRD-061

## What this template produces

A Java project preconfigured with the Bordo data-access layer:
- HikariCP connection pool
- Flyway schema migrations (classpath-based)
- `Repository<T, ID>` base interface for CRUD
- JDBC compatible with any RDBMS (PostgreSQL, MySQL, SQLite, Oracle, ...)
- OTel JDBC tracing instrumentation
- Example migration and repository class

## Variables

| Variable | Description | Default |
|---|---|---|
| `ProjectName` | Project name | required |
| `GroupId` | Maven group ID | `com.example` |
| `DatabaseDriver` | JDBC driver class | `org.postgresql.Driver` |

## Status

🚧 Template files not yet created — see **BRD-061** (Java data-layer issue).

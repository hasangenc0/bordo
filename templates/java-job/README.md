# Template: java-job

**Language**: Java 21  
**Runtime**: Scheduled / cron job  
**Issue**: BRD-060

## What this template produces

A Java scheduled job application with:
- Cron schedule configurable via env var
- Distributed locking (database-backed, prevents duplicate runs)
- Structured JSON logging with job run metadata
- OpenTelemetry auto-instrumentation
- Runs as a Kubernetes CronJob
- Dockerfile (multi-stage)

## Status

🚧 Template files not yet created — see **BRD-060** (Java app framework epic).

# api/proto/bordo/v1/

Protobuf definitions for the Bordo gRPC API.

## Planned proto files (BRD-001 through BRD-031)

| File | Service | Issues |
|---|---|---|
| `bordo.proto` | `BordoService` — root service, Ping, version | BRD-001 |
| `project.proto` | `ProjectService` — CRUD | BRD-002 |
| `build.proto` | `BuildService` — trigger, status, logs | BRD-007 |
| `release.proto` | `ReleaseService` — deploy, rollback, status | BRD-020 |
| `observe.proto` | `ObserveService` — metrics, logs, traces | BRD-031 |
| `fleet.proto` | `FleetService` — region CRUD, node add | BRD-011 |

## Generating Go stubs

```bash
# Install tools
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest

# Generate
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       --grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative \
       api/proto/bordo/v1/*.proto
```

A `make proto` target will be added in BRD-001.

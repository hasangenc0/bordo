# api/openapi/

OpenAPI 3.0 specs generated from the gRPC + grpc-gateway definitions, plus standalone
schemas for non-gRPC surfaces.

## Files (planned)

| File | Description | Issue |
|---|---|---|
| `bordo.yaml` | Main API spec (generated from proto via grpc-gateway) | BRD-001 |
| `template.yaml` | Template manifest schema (used by the template engine) | BRD-005 |

## Generation

The main spec is generated from proto annotations via `protoc-gen-openapiv2`.
`template.yaml` is hand-authored.

A `make openapi` target will be added in BRD-001.

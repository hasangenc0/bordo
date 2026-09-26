// Package server will implement the bordod gRPC + REST gateway server.
//
// Responsibilities (BRD-001):
//   - Start a gRPC listener on :7400
//   - Start a grpc-gateway REST listener on :7401
//   - Auth middleware (bearer token)
//   - Structured request logging with request IDs
//   - Graceful shutdown on SIGINT/SIGTERM
//
// See tracker/todo/BRD-001-control-plane-api-server.md.
package server

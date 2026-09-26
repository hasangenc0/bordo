// Package registry will implement the Bordo project catalog and registry.
//
// Responsibilities (BRD-002):
//   - Project CRUD (create, get, list, delete)
//   - SQLite-backed store with a database/sql interface (swappable to Postgres)
//   - Schema migrations via embedded SQL files
//   - gRPC service handler for ProjectService
//
// See tracker/todo/BRD-002-project-registry.md.
package registry

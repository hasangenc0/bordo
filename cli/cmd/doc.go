// Package cmd will contain the Cobra command implementations for the bordo CLI.
//
// Each file handles one command group:
//   - root.go   — root command + global flags (--server, --token, --output)
//   - config.go — bordo config set/get
//   - login.go  — bordo login
//   - project.go — bordo project create/list/get/delete
//   - build.go  — bordo build trigger/list/logs
//   - region.go — bordo region add/list/status
//   - deploy.go — bordo deploy/status/rollback
//
// See tracker/todo/BRD-004-cli-project-commands.md.
package cmd

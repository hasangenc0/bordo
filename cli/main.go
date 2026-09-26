// Package main is the entry point for bordo, the Bordo CLI.
//
// bordo is the command-line interface for the Bordo software factory platform.
// It provides full feature parity with the AI agent — every operation the agent
// can perform is also available as a CLI command, making it suitable for CI/CD
// pipelines and users who prefer the terminal.
//
// Command tree (BRD-004):
//
//	bordo version
//	bordo config set <key> <value>
//	bordo config get <key>
//	bordo login --server <url> --token <token>
//	bordo project create <name> --template <tpl> [--region <r>]
//	bordo project list [--output json]
//	bordo project get <name>
//	bordo project delete <name>
//	bordo build trigger <project> [--context <dir>]
//	bordo build list <project>
//	bordo build logs <project> [--build-id <id>]
//	bordo region add --name <n> --host <ip> --key <keyfile>
//	bordo region list
//	bordo deploy <project> <version> --region <r>
//	bordo deploy status <project>
//	bordo deploy rollback <project>
//
// See tracker/todo/BRD-004-cli-project-commands.md for the next implementation step.
package main

import (
	"fmt"
	"os"
)

// Version variables are set at build time via -ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("bordo version %s (commit %s, built %s)\n", version, commit, buildTime)
		return
	}

	// TODO(BRD-004): implement Cobra command tree.
	fmt.Fprintf(os.Stderr, "bordo: CLI not yet implemented — see tracker/todo/BRD-004-cli-project-commands.md\n")
	fmt.Fprintf(os.Stderr, "Usage: bordo <command> [flags]\n")
	fmt.Fprintf(os.Stderr, "Commands: version, config, login, project, build, region, deploy\n")
	os.Exit(1)
}

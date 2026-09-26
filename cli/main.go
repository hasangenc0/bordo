// Package main is the entry point for bordo, the Bordo CLI.
//
// See cmd/ for the full command tree.
package main

import (
	"os"

	"github.com/bordo-io/bordo/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

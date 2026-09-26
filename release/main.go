package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "bordo-release: run as a library embedded in bordod, not standalone")
	os.Exit(1)
}

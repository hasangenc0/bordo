package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "bordo-observe: embedded in control-plane, not a standalone binary")
	os.Exit(1)
}

// Command loadout manages a curated shell environment.
package main

import (
	"fmt"
	"os"
)

// Version is overridden at build time with -ldflags.
var Version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

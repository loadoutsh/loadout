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
		// A command that has already reported its own findings returns an
		// error with no message, to set the exit code without saying so twice.
		if err.Error() != "" {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

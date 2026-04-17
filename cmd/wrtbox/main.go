// Command wrtbox is the declarative CLI for OpenWrt split-routing and
// anti-censorship setup. See https://github.com/itslavrov/wrtbox.
package main

import (
	"fmt"
	"os"

	"github.com/itslavrov/wrtbox/cmd/wrtbox/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

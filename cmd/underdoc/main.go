// SPDX-License-Identifier: MIT

// Command underdoc is the UnderDoc server. For now it only knows its own version:
// the server arrives with milestone M-1 (see backlog/).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Ollornog/UnderDoc/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(stdout, "underdoc", version.Version)
		return 0
	}
	fmt.Fprintln(stderr, "usage: underdoc version")
	return 2
}

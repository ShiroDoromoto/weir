// Command weir is the gate: it judges commit and push against the rules
// written in the configuration, and holds nothing of its own.
package main

import (
	"os"

	"github.com/ShiroDoromoto/weir/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}

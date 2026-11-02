// Command stratum manages case workspaces, evidence chains and custody chains.
package main

import (
	"os"

	"stratum/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))
}

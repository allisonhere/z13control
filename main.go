package main

// z13center — a modern GTK4 control center for the 2025 ASUS ROG Flow Z13.
// Talks to the z13ctl daemon over its Unix socket via github.com/dahui/z13ctl/api;
// falls back to an in-memory preview when the daemon is not running.

import (
	"fmt"
	"os"

	"github.com/drbayless/z13center/internal/app"
)

// Version is set at link time via -X main.Version=<version>.
var Version = "dev"

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("z13center %s\n", Version)
			return
		}
	}
	os.Exit(app.New().Run(os.Args))
}

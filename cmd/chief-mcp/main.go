// Command chief-mcp serves the Chief public API to external agents as Model
// Context Protocol tools over stdio or Streamable HTTP.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Storytell-ai/chief-mcp/mcp"
)

// version is the release version, injected at build time via
// -ldflags "-X main.version=...", and assigned into mcp.Version.
var version string

func main() {
	mcp.Version = version
	if err := mcp.Execute(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

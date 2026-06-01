// Command chief-mcp serves the Chief public API to external agents as Model
// Context Protocol tools over stdio or Streamable HTTP.
package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if err := Execute(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

package mcp

import (
	"strings"
	"testing"
)

// TestToolAnnotations pins what Claude's connectors directory checks on every
// tool: a title, a name of at most 64 characters, and exactly one of
// readOnlyHint or destructiveHint. Read-only connections will be offered only
// the tools marked read, so the access a tool declares must match what it does;
// the name prefix is the cross-check, since every read tool lists, gets, or
// searches and no write tool does.
func TestToolAnnotations(t *testing.T) {
	for _, tool := range listStdioTools(t) {
		if len(tool.Name) > 64 {
			t.Errorf("%s: name is %d characters, over 64", tool.Name, len(tool.Name))
		}
		if tool.Title == "" {
			t.Errorf("%s: no title", tool.Name)
		}
		a := tool.Annotations
		if a == nil || a.Title == "" {
			t.Errorf("%s: no annotations title", tool.Name)
			continue
		}
		destructive := a.DestructiveHint != nil && *a.DestructiveHint
		if a.ReadOnlyHint == destructive {
			t.Errorf("%s: readOnlyHint %t, destructiveHint %t; want exactly one true", tool.Name, a.ReadOnlyHint, destructive)
		}
		looksRead := strings.HasPrefix(tool.Name, "list_") || strings.HasPrefix(tool.Name, "get_") || strings.HasPrefix(tool.Name, "search_")
		if a.ReadOnlyHint != looksRead {
			t.Errorf("%s: readOnlyHint %t disagrees with its name", tool.Name, a.ReadOnlyHint)
		}
	}
}

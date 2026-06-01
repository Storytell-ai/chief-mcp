package main

import (
	"context"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var toolGroups = []func(*mcp.Server, *chief.Client){
	registerAssetTools,
	registerLabelTools,
	registerActionTools,
	registerSessionTools,
	registerSkillTools,
	registerMemoryTools,
}

// newServer builds an MCP server bound to one client and registers every tool.
//
// To add a tool group: write a registerXxxTools(s, c) in its own file and add
// it to toolGroups.
func newServer(c *chief.Client) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "chief-mcp", Version: buildVersion()}, nil)
	for _, register := range toolGroups {
		register(s, c)
	}
	return s
}

// toolFunc is a tool's logic: it takes the typed request and returns the typed
// response plus a one-line summary for the text result.
type toolFunc[Req, Resp any] func(ctx context.Context, c *chief.Client, req Req) (Resp, string, error)

type toolMeta struct {
	name string
	desc string
}

// addTool registers fn as an MCP tool, wrapping it with the shared result and
// error plumbing.
func addTool[Req, Resp any](s *mcp.Server, c *chief.Client, meta toolMeta, fn toolFunc[Req, Resp]) {
	mcp.AddTool(s, &mcp.Tool{Name: meta.name, Description: meta.desc},
		func(ctx context.Context, _ *mcp.CallToolRequest, req Req) (*mcp.CallToolResult, Resp, error) {
			resp, summary, err := fn(ctx, c, req)
			if err != nil {
				var zero Resp
				return nil, zero, err
			}
			return textResult(summary), resp, nil
		})
}

// textResult wraps a summary string as a tool result; the SDK derives the
// structured output from the typed return value separately.
func textResult(summary string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: summary}},
	}
}

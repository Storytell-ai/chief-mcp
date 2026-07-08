package mcp

import (
	"context"

	"github.com/Storytell-ai/chief-go/chief"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var toolGroups = []func(*mcpsdk.Server, *chief.Client){
	registerChatTools,
	registerAssetTools,
	registerLabelTools,
	registerActionTools,
	registerSessionTools,
	registerSkillTools,
	registerMemoryTools,
	registerProjectTools,
}

// newServer builds an MCP server bound to one client and registers every tool.
//
// To add a tool group: write a registerXxxTools(s, c) in its own file and add
// it to toolGroups.
func newServer(c *chief.Client) *mcpsdk.Server {
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "chief-mcp", Version: buildVersion()}, nil)
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
func addTool[Req, Resp any](s *mcpsdk.Server, c *chief.Client, meta toolMeta, fn toolFunc[Req, Resp]) {
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: meta.name, Description: meta.desc},
		func(ctx context.Context, _ *mcpsdk.CallToolRequest, req Req) (*mcpsdk.CallToolResult, Resp, error) {
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
func textResult(summary string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: summary}},
	}
}

// Package mcp exposes the Chief public API as Model Context Protocol tools, one
// tool per API operation, over stdio for a local agent or Streamable HTTP for
// the hosted server.
package mcp

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/google/jsonschema-go/jsonschema"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var toolGroups = []func(*toolServer){
	registerChatTools,
	registerAssetTools,
	registerLabelTools,
	registerActionTools,
	registerSessionTools,
	registerSkillTools,
	registerMemoryTools,
	registerProjectTools,
	registerSearchTools,
}

// toolServer is an MCP server under construction and what its tools bind to.
type toolServer struct {
	mcp    *mcpsdk.Server
	client *chief.Client
	// access is the most a listed tool may do: toolRead leaves out every write
	// tool, so a read-only connection never sees one.
	access toolAccess
}

// newServer builds an MCP server bound to one client and registers every tool
// that access allows.
//
// To add a tool group: write a registerXxxTools(s) in its own file and add it
// to toolGroups. A tool whose route needs a project embeds projectInput in its
// request type.
func newServer(c *chief.Client, access toolAccess) *toolServer {
	s := &toolServer{
		mcp:    mcpsdk.NewServer(&mcpsdk.Implementation{Name: "chief-mcp", Version: buildVersion()}, nil),
		client: c,
		access: access,
	}
	for _, register := range toolGroups {
		register(s)
	}
	return s
}

// toolFunc is a tool's logic: it takes the typed request and returns the typed
// response plus a one-line summary for the text result.
type toolFunc[Req, Resp any] func(ctx context.Context, c *chief.Client, req Req) (Resp, string, error)

// toolAccess says whether a tool only reads Chief data or can change it, and
// sets the tool's read-only and destructive hints.
type toolAccess int

// toolRead starts at one so a toolMeta that never set access carries neither
// hint and fails the annotation test.
const (
	toolRead toolAccess = iota + 1
	toolWrite
)

type toolMeta struct {
	name   string
	title  string
	desc   string
	access toolAccess
	// openWorld marks a tool whose effect reaches past Chief: a public URL, an
	// email, or an LLM turn that spends credits.
	openWorld bool
}

// projectInput is embedded in the request of every tool whose API route needs
// a project. Its project_id overrides the connection's project for that call;
// on a connection without one, which is every OAuth connection, the tool's
// schema makes it required.
type projectInput struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"the project to act in; defaults to the connection's project. list_projects returns the IDs"`
}

func (p projectInput) projectOverride() string { return p.ProjectID }

type projectScoped interface {
	projectOverride() string
}

// addTool registers fn as an MCP tool, wrapping it with the shared result and
// error plumbing. A write tool is skipped when s.access is toolRead.
func addTool[Req, Resp any](s *toolServer, meta toolMeta, fn toolFunc[Req, Resp]) {
	if meta.access == toolWrite && s.access == toolRead {
		return
	}
	// openWorldHint is always set: the spec reads an absent value as true.
	annotations := &mcpsdk.ToolAnnotations{
		Title:         meta.title,
		ReadOnlyHint:  meta.access == toolRead,
		OpenWorldHint: new(meta.openWorld),
	}
	if meta.access == toolWrite {
		annotations.DestructiveHint = new(true)
	}
	tool := &mcpsdk.Tool{Name: meta.name, Title: meta.title, Description: meta.desc, Annotations: annotations}
	// The SDK validates arguments against the schema, so a call without a
	// project is refused before it reaches the handler.
	if _, scoped := any(*new(Req)).(projectScoped); scoped && s.client.ProjectID() == "" {
		schema, err := jsonschema.For[Req](&jsonschema.ForOptions{})
		if err != nil {
			panic(fmt.Errorf("infer %s input schema: %w", meta.name, err))
		}
		project := schema.Properties["project_id"]
		project.Description = "the project to act in; list_projects returns the IDs"
		project.MinLength = jsonschema.Ptr(1)
		schema.Required = append(schema.Required, "project_id")
		tool.InputSchema = schema
	}
	mcpsdk.AddTool(s.mcp, tool,
		func(ctx context.Context, _ *mcpsdk.CallToolRequest, req Req) (*mcpsdk.CallToolResult, Resp, error) {
			var zero Resp
			c := s.client
			if scoped, ok := any(req).(projectScoped); ok {
				if project := scoped.projectOverride(); project != "" {
					c = c.ForProject(project)
				}
			}
			resp, summary, err := fn(ctx, c, req)
			if err != nil {
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

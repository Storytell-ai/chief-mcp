// Package mcp exposes the Chief public API as Model Context Protocol tools, one
// tool per API operation, over stdio for a local agent or Streamable HTTP for
// the hosted server.
package mcp

import (
	"context"
	"encoding/json"
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
	registerCaptureTools,
	registerLinkTools,
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
// request type. Register a tool with addTool, or with addContentTool when its
// answer is images or text written for the model.
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
// response plus the text result.
type toolFunc[Req, Resp any] func(ctx context.Context, c *chief.Client, req Req) (Resp, string, error)

// contentFunc is the logic of a tool whose whole answer is its content.
type contentFunc[Req any] func(ctx context.Context, c *chief.Client, req Req) ([]mcpsdk.Content, error)

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

// addTool registers fn as an MCP tool that answers with its summary and its
// response as text, and its response as structured output too: some clients
// show their model only the text, and some only the structured output.
func addTool[Req, Resp any](s *toolServer, meta toolMeta, fn toolFunc[Req, Resp]) {
	registerTool(s, meta, func(ctx context.Context, c *chief.Client, req Req) (*mcpsdk.CallToolResult, Resp, error) {
		resp, summary, err := fn(ctx, c, req)
		if err != nil {
			var zero Resp
			return nil, zero, err
		}
		data, err := json.Marshal(resp)
		if err != nil {
			var zero Resp
			return nil, zero, fmt.Errorf("encode response: %w", err)
		}
		return &mcpsdk.CallToolResult{Content: append(textContent(summary), textContent(string(data))...)}, resp, nil
	})
}

// addContentTool registers fn as an MCP tool that answers with content alone.
// It has no structured output: a client that shows its model that in place of
// the content would lose the images.
func addContentTool[Req any](s *toolServer, meta toolMeta, fn contentFunc[Req]) {
	registerTool(s, meta, func(ctx context.Context, c *chief.Client, req Req) (*mcpsdk.CallToolResult, any, error) {
		content, err := fn(ctx, c, req)
		if err != nil {
			return nil, nil, err
		}
		return &mcpsdk.CallToolResult{Content: content}, nil, nil
	})
}

// registerTool adds the tool meta describes, calling it with the client for the
// project its request names. A write tool is skipped when s.access is toolRead.
func registerTool[Req, Resp any](
	s *toolServer, meta toolMeta,
	call func(ctx context.Context, c *chief.Client, req Req) (*mcpsdk.CallToolResult, Resp, error),
) {
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
			c := s.client
			if scoped, ok := any(req).(projectScoped); ok {
				if project := scoped.projectOverride(); project != "" {
					c = c.ForProject(project)
				}
			}
			return call(ctx, c, req)
		})
}

func textContent(text string) []mcpsdk.Content {
	return []mcpsdk.Content{&mcpsdk.TextContent{Text: text}}
}

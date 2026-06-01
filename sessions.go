package main

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolListSessions  = "list_sessions"
	toolGetSession    = "get_session"
	toolUpdateSession = "update_session"
	toolDeleteSession = "delete_session"
)

type listSessionsRequest struct {
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of sessions to return; the server clamps to its own bounds"`
	AfterID  string `json:"after_id,omitempty" jsonschema:"page forward from this session ID"`
	BeforeID string `json:"before_id,omitempty" jsonschema:"page backward from this session ID"`
}

type sessionIDRequest struct {
	SessionID string `json:"session_id" jsonschema:"the session ID"`
}

type updateSessionRequest struct {
	SessionID string                     `json:"session_id" jsonschema:"the session to update"`
	Session   chief.UpdateSessionRequest `json:"session" jsonschema:"name and/or description patch; omitted fields are left unchanged"`
}

type deleteSessionResponse struct {
	Deleted bool `json:"deleted"`
}

func registerSessionTools(s *mcp.Server, c *chief.Client) {
	addTool(s, c, toolMeta{
		name: toolListSessions,
		desc: "List the caller's sessions in the project, newest first, cursor-paginated. Use after_id / before_id with the returned first_id / last_id to page.",
	}, listSessions)
	addTool(s, c, toolMeta{
		name: toolGetSession,
		desc: "Get a single session by ID, including its full transcript.",
	}, getSession)
	addTool(s, c, toolMeta{
		name: toolUpdateSession,
		desc: "Patch a session's name and/or description. Omitted fields are left unchanged.",
	}, updateSession)
	addTool(s, c, toolMeta{
		name: toolDeleteSession,
		desc: "Delete a session permanently.",
	}, deleteSession)
}

func listSessions(ctx context.Context, c *chief.Client, req listSessionsRequest) (*chief.SessionPage, string, error) {
	var opts []chief.ListOption
	if req.Limit > 0 {
		opts = append(opts, chief.WithLimit(req.Limit))
	}
	if req.AfterID != "" {
		opts = append(opts, chief.WithAfterID(req.AfterID))
	}
	if req.BeforeID != "" {
		opts = append(opts, chief.WithBeforeID(req.BeforeID))
	}

	list, err := c.Sessions.List(ctx, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("list sessions: %w", err)
	}
	return list, fmt.Sprintf("%d session(s) returned (has_more %t)", len(list.Data), list.HasMore), nil
}

func getSession(ctx context.Context, c *chief.Client, req sessionIDRequest) (*chief.SessionResponse, string, error) {
	session, err := c.Sessions.Get(ctx, req.SessionID)
	if err != nil {
		return nil, "", fmt.Errorf("get session %q: %w", req.SessionID, err)
	}
	return session, fmt.Sprintf("session %s: %s (%d turn(s))", session.SessionID, session.Name, len(session.Turns)), nil
}

func updateSession(ctx context.Context, c *chief.Client, req updateSessionRequest) (*chief.SessionResponse, string, error) {
	session, err := c.Sessions.Update(ctx, req.SessionID, &req.Session)
	if err != nil {
		return nil, "", fmt.Errorf("update session %q: %w", req.SessionID, err)
	}
	return session, fmt.Sprintf("updated session %s (%s)", session.Name, session.SessionID), nil
}

func deleteSession(ctx context.Context, c *chief.Client, req sessionIDRequest) (deleteSessionResponse, string, error) {
	if err := c.Sessions.Delete(ctx, req.SessionID); err != nil {
		return deleteSessionResponse{}, "", fmt.Errorf("delete session %q: %w", req.SessionID, err)
	}
	return deleteSessionResponse{Deleted: true}, fmt.Sprintf("deleted session %s", req.SessionID), nil
}

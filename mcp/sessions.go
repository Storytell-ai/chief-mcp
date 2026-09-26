package mcp

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolListSessions         = "list_sessions"
	toolGetSession           = "get_session"
	toolGetSessionTranscript = "get_session_transcript"
	toolUpdateSession        = "update_session"
	toolDeleteSession        = "delete_session"
)

type listSessionsRequest struct {
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of sessions to return; the server clamps to its own bounds"`
	AfterID  string `json:"after_id,omitempty" jsonschema:"page forward from this session ID"`
	BeforeID string `json:"before_id,omitempty" jsonschema:"page backward from this session ID"`
}

type sessionIDRequest struct {
	SessionID string `json:"session_id" jsonschema:"the session ID"`
}

type getSessionTranscriptRequest struct {
	SessionID string `json:"session_id" jsonschema:"the session ID"`
	Limit     int    `json:"limit,omitempty" jsonschema:"turns per page, 1 to 100; defaults to 25"`
	AfterID   string `json:"after_id,omitempty" jsonschema:"the previous page's last_id; returns the turns after that index"`
	BeforeID  string `json:"before_id,omitempty" jsonschema:"a turn index, or the session's turn_count for the most recent turns; returns the turns just before it, still oldest first"`
}

type updateSessionRequest struct {
	SessionID string                     `json:"session_id" jsonschema:"the session to update"`
	Session   chief.UpdateSessionRequest `json:"session" jsonschema:"name and/or description patch; omitted fields are left unchanged"`
}

type deleteSessionResponse struct {
	Deleted bool `json:"deleted"`
}

func registerSessionTools(s *mcpsdk.Server, c *chief.Client) {
	addTool(s, c, toolMeta{
		name: toolListSessions,
		desc: "List the caller's sessions in the project, newest first, cursor-paginated. Each item carries the session's lifecycle state, so finished sessions (state session.ended) can be picked out without fetching each one. Use after_id / before_id with the returned first_id / last_id to page.",
	}, listSessions)
	addTool(s, c, toolMeta{
		name: toolGetSession,
		desc: "Get a single session by ID: its lifecycle state, live summary, and turn_count. Use it for what a session decided and what came out of it; for what was actually said (quotes, who said what, when), use get_session_transcript. state distinguishes a finished session (session.ended) from one still scheduled or running. The live summary is the canonical record of what the session decided: its items with kind todo are the follow-ups. The turns field is deprecated and will be removed; read the transcript with get_session_transcript.",
	}, getSession)
	addTool(s, c, toolMeta{
		name: toolGetSessionTranscript,
		desc: "Read one page of a session's transcript, oldest turn first: the words actually spoken, each turn with its index, speaker label, text, and m:ss timecode when the recording has one. Use it for quotes, who said what, or when something came up; use get_session for the outcome (live summary, todos). With no cursor it starts at the beginning of the meeting; before_id set to the session's turn_count (from get_session) returns the most recent turns. While has_more is true, pass last_id as after_id to read forward, or first_id as before_id to read backward. Pages hold up to 100 turns. Indices are stable once the session has ended (state session.ended); while it is still recording, turns merged from another device can shift them.",
	}, getSessionTranscript)
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
	return session, fmt.Sprintf("session %s: %s (%s, %d turn(s))", session.SessionID, session.Name, session.State.State, session.TurnCount), nil
}

func getSessionTranscript(ctx context.Context, c *chief.Client, req getSessionTranscriptRequest) (*chief.SessionTranscriptPage, string, error) {
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

	page, err := c.Sessions.GetTranscript(ctx, req.SessionID, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("get session %q transcript: %w", req.SessionID, err)
	}
	if len(page.Data) == 0 {
		return page, fmt.Sprintf("session %s: no turns (has_more %t)", req.SessionID, page.HasMore), nil
	}
	return page, fmt.Sprintf("session %s: turns %s–%s, %d turn(s) (has_more %t)", req.SessionID, page.FirstID, page.LastID, len(page.Data), page.HasMore), nil
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

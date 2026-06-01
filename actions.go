package main

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolCreateAction  = "create_action"
	toolListActions   = "list_actions"
	toolGetAction     = "get_action"
	toolUpdateAction  = "update_action"
	toolDeleteAction  = "delete_action"
	toolEnableAction  = "enable_action"
	toolDisableAction = "disable_action"
)

type actionIDRequest struct {
	ActionID string `json:"action_id" jsonschema:"the action ID"`
}

type updateActionRequest struct {
	ActionID string              `json:"action_id" jsonschema:"the action to update"`
	Action   chief.ActionRequest `json:"action" jsonschema:"the full replacement action body; omitted schedule, trigger, scope, or email sections are cleared"`
}

type deleteActionResponse struct {
	Deleted bool `json:"deleted"`
}

func registerActionTools(s *mcp.Server, c *chief.Client) {
	addTool(s, c, toolMeta{
		name: toolCreateAction,
		desc: "Create an action in the project. Prompt is plain text. Provide a schedule for recurring runs or a trigger for event-driven runs. An action always starts enabled.",
	}, createAction)
	addTool(s, c, toolMeta{
		name: toolListActions,
		desc: "List every action in the project.",
	}, listActions)
	addTool(s, c, toolMeta{
		name: toolGetAction,
		desc: "Get a single action by ID, including its full schedule, trigger, scope, and email configuration.",
	}, getAction)
	addTool(s, c, toolMeta{
		name: toolUpdateAction,
		desc: "Replace an action's configuration. The body is a full replacement: any schedule, trigger, scope, or email section omitted from action is cleared.",
	}, updateAction)
	addTool(s, c, toolMeta{
		name: toolDeleteAction,
		desc: "Delete an action permanently.",
	}, deleteAction)
	addTool(s, c, toolMeta{
		name: toolEnableAction,
		desc: "Enable an action so it runs on its schedule or trigger.",
	}, enableAction)
	addTool(s, c, toolMeta{
		name: toolDisableAction,
		desc: "Disable an action without deleting it, pausing its schedule or trigger.",
	}, disableAction)
}

func createAction(ctx context.Context, c *chief.Client, req chief.ActionRequest) (*chief.ActionResponse, string, error) {
	action, err := c.Actions.Create(ctx, &req)
	if err != nil {
		return nil, "", fmt.Errorf("create action %q: %w", req.Name, err)
	}
	return action, fmt.Sprintf("created action %s (%s)", action.Name, action.ActionID), nil
}

func listActions(ctx context.Context, c *chief.Client, _ struct{}) (*chief.ActionPage, string, error) {
	page, err := c.Actions.List(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("list actions: %w", err)
	}
	return page, fmt.Sprintf("%d action(s) returned", len(page.Data)), nil
}

func getAction(ctx context.Context, c *chief.Client, req actionIDRequest) (*chief.ActionResponse, string, error) {
	action, err := c.Actions.Get(ctx, req.ActionID)
	if err != nil {
		return nil, "", fmt.Errorf("get action %q: %w", req.ActionID, err)
	}
	return action, fmt.Sprintf("action %s: %s (enabled %t)", action.ActionID, action.Name, action.Enabled), nil
}

func updateAction(ctx context.Context, c *chief.Client, req updateActionRequest) (*chief.ActionResponse, string, error) {
	action, err := c.Actions.Update(ctx, req.ActionID, &req.Action)
	if err != nil {
		return nil, "", fmt.Errorf("update action %q: %w", req.ActionID, err)
	}
	return action, fmt.Sprintf("updated action %s (%s)", action.Name, action.ActionID), nil
}

func deleteAction(ctx context.Context, c *chief.Client, req actionIDRequest) (deleteActionResponse, string, error) {
	if err := c.Actions.Delete(ctx, req.ActionID); err != nil {
		return deleteActionResponse{}, "", fmt.Errorf("delete action %q: %w", req.ActionID, err)
	}
	return deleteActionResponse{Deleted: true}, fmt.Sprintf("deleted action %s", req.ActionID), nil
}

func enableAction(ctx context.Context, c *chief.Client, req actionIDRequest) (*chief.ActionResponse, string, error) {
	action, err := c.Actions.Enable(ctx, req.ActionID)
	if err != nil {
		return nil, "", fmt.Errorf("enable action %q: %w", req.ActionID, err)
	}
	return action, fmt.Sprintf("enabled action %s", action.ActionID), nil
}

func disableAction(ctx context.Context, c *chief.Client, req actionIDRequest) (*chief.ActionResponse, string, error) {
	action, err := c.Actions.Disable(ctx, req.ActionID)
	if err != nil {
		return nil, "", fmt.Errorf("disable action %q: %w", req.ActionID, err)
	}
	return action, fmt.Sprintf("disabled action %s", action.ActionID), nil
}

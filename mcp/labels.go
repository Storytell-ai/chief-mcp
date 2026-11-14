package mcp

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
)

const (
	toolCreateLabel = "create_label"
	toolListLabels  = "list_labels"
	toolGetLabel    = "get_label"
	toolUpdateLabel = "update_label"
	toolDeleteLabel = "delete_label"
	toolAttachLabel = "attach_label"
	toolDetachLabel = "detach_label"
)

type createLabelRequest struct {
	projectInput
	chief.CreateLabelRequest
}

type listLabelsRequest struct {
	projectInput
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of labels to return; the server clamps to its own bounds"`
	AfterID  string `json:"after_id,omitempty" jsonschema:"page forward from this label ID"`
	BeforeID string `json:"before_id,omitempty" jsonschema:"page backward from this label ID"`
}

type attachLabelRequest struct {
	projectInput
	AssetID   string `json:"asset_id" jsonschema:"the asset to attach the label to"`
	LabelName string `json:"label_name" jsonschema:"the label name; a name with no matching label is auto-created"`
}

type detachLabelRequest struct {
	projectInput
	AssetID string `json:"asset_id" jsonschema:"the asset to detach the label from"`
	LabelID string `json:"label_id" jsonschema:"the label to detach"`
}

type labelIDRequest struct {
	projectInput
	LabelID string `json:"label_id" jsonschema:"the label ID"`
}

type updateLabelRequest struct {
	projectInput
	LabelID string                   `json:"label_id" jsonschema:"the label to update"`
	Label   chief.UpdateLabelRequest `json:"label" jsonschema:"the patch; omitted fields are left unchanged. color, when set, must be a 6-digit hex code like #6b7280"`
}

type deleteLabelResponse struct {
	Deleted bool `json:"deleted"`
}

type detachLabelResponse struct {
	Detached bool `json:"detached"`
}

func registerLabelTools(s *toolServer) {
	addTool(s, toolMeta{
		name:   toolCreateLabel,
		title:  "Create Label",
		access: toolWrite,
		desc:   "Create a label in the project. Color, when set, must be a 6-digit hex code like #6b7280.",
	}, createLabel)
	addTool(s, toolMeta{
		name:   toolListLabels,
		title:  "List Labels",
		access: toolRead,
		desc:   "List the labels visible in the project, cursor-paginated. Use after_id / before_id with the returned first_id / last_id to page.",
	}, listLabels)
	addTool(s, toolMeta{
		name:   toolGetLabel,
		title:  "Get Label",
		access: toolRead,
		desc:   "Get a single label by ID, including its color and icon.",
	}, getLabel)
	addTool(s, toolMeta{
		name:   toolUpdateLabel,
		title:  "Update Label",
		access: toolWrite,
		desc:   "Patch a label's display metadata. Omitted fields are left unchanged. Color, when set, must be a 6-digit hex code like #6b7280.",
	}, updateLabel)
	addTool(s, toolMeta{
		name:   toolDeleteLabel,
		title:  "Delete Label",
		access: toolWrite,
		desc:   "Delete a label permanently. It is detached from every asset it was attached to.",
	}, deleteLabel)
	addTool(s, toolMeta{
		name:   toolAttachLabel,
		title:  "Attach Label",
		access: toolWrite,
		desc:   "Attach a label by name to an asset. Re-attaching an already-attached label is a no-op.",
	}, attachLabel)
	addTool(s, toolMeta{
		name:   toolDetachLabel,
		title:  "Detach Label",
		access: toolWrite,
		desc:   "Detach a label from an asset by ID. Detaching a label that is not attached is a no-op.",
	}, detachLabel)
}

func createLabel(ctx context.Context, c *chief.Client, req createLabelRequest) (*chief.LabelSummary, string, error) {
	label, err := c.Labels.Create(ctx, &req.CreateLabelRequest)
	if err != nil {
		return nil, "", fmt.Errorf("create label %q: %w", req.Name, err)
	}
	return label, fmt.Sprintf("created label %s (%s)", label.Name, label.LabelID), nil
}

func listLabels(ctx context.Context, c *chief.Client, req listLabelsRequest) (*chief.LabelPage, string, error) {
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

	page, err := c.Labels.List(ctx, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("list labels: %w", err)
	}
	return page, fmt.Sprintf("%d label(s) returned (has_more %t)", len(page.Data), page.HasMore), nil
}

func getLabel(ctx context.Context, c *chief.Client, req labelIDRequest) (*chief.LabelResponse, string, error) {
	label, err := c.Labels.Get(ctx, req.LabelID)
	if err != nil {
		return nil, "", fmt.Errorf("get label %q: %w", req.LabelID, err)
	}
	return label, fmt.Sprintf("label %s: %s", label.LabelID, label.Name), nil
}

func updateLabel(ctx context.Context, c *chief.Client, req updateLabelRequest) (*chief.LabelResponse, string, error) {
	label, err := c.Labels.Update(ctx, req.LabelID, &req.Label)
	if err != nil {
		return nil, "", fmt.Errorf("update label %q: %w", req.LabelID, err)
	}
	return label, fmt.Sprintf("updated label %s (%s)", label.Name, label.LabelID), nil
}

func deleteLabel(ctx context.Context, c *chief.Client, req labelIDRequest) (deleteLabelResponse, string, error) {
	if err := c.Labels.Delete(ctx, req.LabelID); err != nil {
		return deleteLabelResponse{}, "", fmt.Errorf("delete label %q: %w", req.LabelID, err)
	}
	return deleteLabelResponse{Deleted: true}, fmt.Sprintf("deleted label %s", req.LabelID), nil
}

func attachLabel(ctx context.Context, c *chief.Client, req attachLabelRequest) (*chief.LabelSummary, string, error) {
	label, err := c.Assets.AttachLabel(ctx, req.AssetID, req.LabelName)
	if err != nil {
		return nil, "", fmt.Errorf("attach label %q to asset %q: %w", req.LabelName, req.AssetID, err)
	}
	return label, fmt.Sprintf("attached %s to asset %s", label.Name, req.AssetID), nil
}

func detachLabel(ctx context.Context, c *chief.Client, req detachLabelRequest) (detachLabelResponse, string, error) {
	if err := c.Assets.DetachLabel(ctx, req.AssetID, req.LabelID); err != nil {
		return detachLabelResponse{}, "", fmt.Errorf("detach label %q from asset %q: %w", req.LabelID, req.AssetID, err)
	}
	return detachLabelResponse{Detached: true}, fmt.Sprintf("detached label %s from asset %s", req.LabelID, req.AssetID), nil
}

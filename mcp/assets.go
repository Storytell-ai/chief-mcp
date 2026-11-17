package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/Storytell-ai/chief-go/chief"
)

const (
	toolUploadFile  = "upload_file"
	toolListAssets  = "list_assets"
	toolGetAsset    = "get_asset"
	toolUpdateAsset = "update_asset"
	toolDeleteAsset = "delete_asset"
)

const defaultUploadTimeout = 360 * time.Second

type uploadFileRequest struct {
	projectInput
	Path           string `json:"path" jsonschema:"absolute path to a local file, resolved on the host running this MCP server"`
	WaitForReady   bool   `json:"wait_for_ready,omitempty" jsonschema:"block until the asset finishes ingesting before returning"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"seconds to wait when wait_for_ready is set; defaults to 360"`
}

type uploadFileResponse struct {
	AssetID       string            `json:"asset_id"`
	Status        chief.AssetStatus `json:"status"`
	Filename      string            `json:"filename"`
	MimeType      string            `json:"mime_type"`
	SizeInBytes   int64             `json:"size_in_bytes"`
	AlreadyExists bool              `json:"already_exists"`
}

type listAssetsRequest struct {
	projectInput
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of assets to return; the server clamps to its own bounds"`
	AfterID  string `json:"after_id,omitempty" jsonschema:"page forward from this asset ID"`
	BeforeID string `json:"before_id,omitempty" jsonschema:"page backward from this asset ID"`
}

type getAssetRequest struct {
	projectInput
	AssetID string `json:"asset_id" jsonschema:"the asset ID to fetch"`
}

type updateAssetRequest struct {
	projectInput
	AssetID     string  `json:"asset_id" jsonschema:"the asset to update"`
	Name        *string `json:"name,omitempty" jsonschema:"new display name; omit to leave unchanged"`
	Description *string `json:"description,omitempty" jsonschema:"new description; omit to leave unchanged"`
}

type deleteAssetRequest struct {
	projectInput
	AssetID string `json:"asset_id" jsonschema:"the asset to delete"`
}

type deleteAssetResponse struct {
	Deleted bool `json:"deleted"`
}

// registerUploadTool is for stdio only. upload_file opens a path on this
// process's filesystem, which is the caller's own machine only on stdio; on the
// hosted HTTP transport it would read the server's files, its service account
// token included.
func registerUploadTool(s *toolServer) {
	addTool(s, toolMeta{
		name:   toolUploadFile,
		title:  "Upload File",
		access: toolWrite,
		desc:   "Upload a file from the machine running this MCP server as an asset. Returns the asset and whether its content already existed (a dedup hit).",
	}, uploadFile)
}

func registerAssetTools(s *toolServer) {
	addTool(s, toolMeta{
		name:   toolListAssets,
		title:  "List Assets",
		access: toolRead,
		desc:   "List assets in the project, cursor-paginated. Use after_id / before_id with the returned first_id / last_id to page.",
	}, listAssets)
	addTool(s, toolMeta{
		name:   toolGetAsset,
		title:  "Get Asset",
		access: toolRead,
		desc:   "Get a single asset by ID, including its current ingest status.",
	}, getAsset)
	addTool(s, toolMeta{
		name:   toolUpdateAsset,
		title:  "Update Asset",
		access: toolWrite,
		desc:   "Patch an asset's name and/or description. Omitted fields are left unchanged.",
	}, updateAsset)
	addTool(s, toolMeta{
		name:   toolDeleteAsset,
		title:  "Delete Asset",
		access: toolWrite,
		desc:   "Delete an asset permanently.",
	}, deleteAsset)
}

func uploadFile(ctx context.Context, c *chief.Client, req uploadFileRequest) (uploadFileResponse, string, error) {
	asset, alreadyExists, err := c.Assets.UploadFile(ctx, req.Path)
	if err != nil {
		return uploadFileResponse{}, "", fmt.Errorf("upload file %q: %w", req.Path, err)
	}

	if req.WaitForReady && !alreadyExists {
		timeout := defaultUploadTimeout
		if req.TimeoutSeconds > 0 {
			timeout = time.Duration(req.TimeoutSeconds) * time.Second
		}
		ready, err := c.Assets.WaitForReady(ctx, asset.AssetID, timeout)
		if err != nil {
			return uploadFileResponse{}, "", fmt.Errorf("wait for asset %s: %w", asset.AssetID, err)
		}
		asset = ready
	}

	out := uploadFileResponse{
		AssetID:       asset.AssetID,
		Status:        asset.Status,
		Filename:      asset.Filename,
		MimeType:      asset.MimeType,
		SizeInBytes:   asset.SizeInBytes,
		AlreadyExists: alreadyExists,
	}
	summary := fmt.Sprintf("uploaded %s as asset %s (status %s)", asset.Filename, asset.AssetID, asset.Status)
	if alreadyExists {
		summary = fmt.Sprintf("asset %s already existed for %s (status %s)", asset.AssetID, asset.Filename, asset.Status)
	}
	return out, summary, nil
}

func listAssets(ctx context.Context, c *chief.Client, req listAssetsRequest) (*chief.AssetPage, string, error) {
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

	page, err := c.Assets.List(ctx, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("list assets: %w", err)
	}
	return page, fmt.Sprintf("%d asset(s) returned (has_more %t)", len(page.Data), page.HasMore), nil
}

func getAsset(ctx context.Context, c *chief.Client, req getAssetRequest) (*chief.Asset, string, error) {
	asset, err := c.Assets.Get(ctx, req.AssetID)
	if err != nil {
		return nil, "", fmt.Errorf("get asset %q: %w", req.AssetID, err)
	}
	return asset, fmt.Sprintf("asset %s: %s (status %s)", asset.AssetID, asset.Filename, asset.Status), nil
}

func updateAsset(ctx context.Context, c *chief.Client, req updateAssetRequest) (*chief.Asset, string, error) {
	asset, err := c.Assets.Update(ctx, req.AssetID, &chief.UpdateAssetRequest{Name: req.Name, Description: req.Description})
	if err != nil {
		return nil, "", fmt.Errorf("update asset %q: %w", req.AssetID, err)
	}
	return asset, fmt.Sprintf("updated asset %s: %s", asset.AssetID, asset.Filename), nil
}

func deleteAsset(ctx context.Context, c *chief.Client, req deleteAssetRequest) (deleteAssetResponse, string, error) {
	if err := c.Assets.Delete(ctx, req.AssetID); err != nil {
		return deleteAssetResponse{}, "", fmt.Errorf("delete asset %q: %w", req.AssetID, err)
	}
	return deleteAssetResponse{Deleted: true}, fmt.Sprintf("deleted asset %s", req.AssetID), nil
}

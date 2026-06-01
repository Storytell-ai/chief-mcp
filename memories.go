package main

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolCreateMemory = "create_memory"
	toolListMemories = "list_memories"
	toolGetMemory    = "get_memory"
	toolUpdateMemory = "update_memory"
	toolDeleteMemory = "delete_memory"
)

type listMemoriesRequest struct {
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of memories to return; the server clamps to its own bounds"`
	AfterID  string `json:"after_id,omitempty" jsonschema:"page forward from this memory ID"`
	BeforeID string `json:"before_id,omitempty" jsonschema:"page backward from this memory ID"`
}

type memoryIDRequest struct {
	MemoryID string `json:"memory_id" jsonschema:"the memory ID"`
}

type updateMemoryRequest struct {
	MemoryID string                    `json:"memory_id" jsonschema:"the memory to update"`
	Memory   chief.UpdateMemoryRequest `json:"memory" jsonschema:"content is replaced; category and importance are optional patches. category is one of: identity, preference, fact, context, instruction"`
}

type deleteMemoryResponse struct {
	Deleted bool `json:"deleted"`
}

func registerMemoryTools(s *mcp.Server, c *chief.Client) {
	addTool(s, c, toolMeta{
		name: toolCreateMemory,
		desc: "Store a memory in the project. category is one of: identity, preference, fact, context, instruction. importance is an integer. scope is optional and, for now, only accepts \"project\".",
	}, createMemory)
	addTool(s, c, toolMeta{
		name: toolListMemories,
		desc: "List the memories in the project, cursor-paginated. Use after_id / before_id with the returned first_id / last_id to page.",
	}, listMemories)
	addTool(s, c, toolMeta{
		name: toolGetMemory,
		desc: "Get a single memory by ID.",
	}, getMemory)
	addTool(s, c, toolMeta{
		name: toolUpdateMemory,
		desc: "Update a memory. content is replaced; category and importance are optional patches. category is one of: identity, preference, fact, context, instruction.",
	}, updateMemory)
	addTool(s, c, toolMeta{
		name: toolDeleteMemory,
		desc: "Delete a memory permanently.",
	}, deleteMemory)
}

func createMemory(ctx context.Context, c *chief.Client, req chief.CreateMemoryRequest) (*chief.MemoryResponse, string, error) {
	memory, err := c.Memories.Create(ctx, &req)
	if err != nil {
		return nil, "", fmt.Errorf("create memory: %w", err)
	}
	return memory, fmt.Sprintf("created memory %s (%s)", memory.MemoryID, memory.Category), nil
}

func listMemories(ctx context.Context, c *chief.Client, req listMemoriesRequest) (*chief.MemoryPage, string, error) {
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

	page, err := c.Memories.List(ctx, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("list memories: %w", err)
	}
	return page, fmt.Sprintf("%d memory(ies) returned (has_more %t)", len(page.Data), page.HasMore), nil
}

func getMemory(ctx context.Context, c *chief.Client, req memoryIDRequest) (*chief.MemoryResponse, string, error) {
	memory, err := c.Memories.Get(ctx, req.MemoryID)
	if err != nil {
		return nil, "", fmt.Errorf("get memory %q: %w", req.MemoryID, err)
	}
	return memory, fmt.Sprintf("memory %s (%s, importance %d)", memory.MemoryID, memory.Category, memory.Importance), nil
}

func updateMemory(ctx context.Context, c *chief.Client, req updateMemoryRequest) (*chief.MemoryResponse, string, error) {
	memory, err := c.Memories.Update(ctx, req.MemoryID, &req.Memory)
	if err != nil {
		return nil, "", fmt.Errorf("update memory %q: %w", req.MemoryID, err)
	}
	return memory, fmt.Sprintf("updated memory %s (%s)", memory.MemoryID, memory.Category), nil
}

func deleteMemory(ctx context.Context, c *chief.Client, req memoryIDRequest) (deleteMemoryResponse, string, error) {
	if err := c.Memories.Delete(ctx, req.MemoryID); err != nil {
		return deleteMemoryResponse{}, "", fmt.Errorf("delete memory %q: %w", req.MemoryID, err)
	}
	return deleteMemoryResponse{Deleted: true}, fmt.Sprintf("deleted memory %s", req.MemoryID), nil
}

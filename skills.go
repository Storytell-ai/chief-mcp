package main

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolCreateSkill  = "create_skill"
	toolListSkills   = "list_skills"
	toolGetSkill     = "get_skill"
	toolUpdateSkill  = "update_skill"
	toolDeleteSkill  = "delete_skill"
	toolEnableSkill  = "enable_skill"
	toolDisableSkill = "disable_skill"
)

type listSkillsRequest struct {
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of skills to return; the server clamps to its own bounds"`
	AfterID  string `json:"after_id,omitempty" jsonschema:"page forward from this skill ID"`
	BeforeID string `json:"before_id,omitempty" jsonschema:"page backward from this skill ID"`
}

type skillIDRequest struct {
	SkillID string `json:"skill_id" jsonschema:"the skill ID"`
}

type updateSkillRequest struct {
	SkillID string                   `json:"skill_id" jsonschema:"the skill to update"`
	Skill   chief.UpdateSkillRequest `json:"skill" jsonschema:"the patch; omitted fields are left unchanged. category, when set, must be \"skill\" or \"persona\""`
}

type deleteSkillResponse struct {
	Deleted bool `json:"deleted"`
}

func registerSkillTools(s *mcp.Server, c *chief.Client) {
	addTool(s, c, toolMeta{
		name: toolCreateSkill,
		desc: "Create a skill in the project. scope is \"project\" or \"user\". category is \"skill\" or \"persona\". Content is the skill body.",
	}, createSkill)
	addTool(s, c, toolMeta{
		name: toolListSkills,
		desc: "List the skills visible in the project, cursor-paginated, including each skill's enabled state for the caller. Use after_id / before_id with the returned first_id / last_id to page.",
	}, listSkills)
	addTool(s, c, toolMeta{
		name: toolGetSkill,
		desc: "Get a single skill by ID, including its content and enabled state for the caller.",
	}, getSkill)
	addTool(s, c, toolMeta{
		name: toolUpdateSkill,
		desc: "Patch a skill. Omitted fields are left unchanged. category, when set, must be \"skill\" or \"persona\". System skills are read-only.",
	}, updateSkill)
	addTool(s, c, toolMeta{
		name: toolDeleteSkill,
		desc: "Delete a skill permanently. System skills are read-only.",
	}, deleteSkill)
	addTool(s, c, toolMeta{
		name: toolEnableSkill,
		desc: "Enable a skill for the caller so it is offered during chats.",
	}, enableSkill)
	addTool(s, c, toolMeta{
		name: toolDisableSkill,
		desc: "Disable a skill for the caller so it is no longer offered during chats.",
	}, disableSkill)
}

func createSkill(ctx context.Context, c *chief.Client, req chief.CreateSkillRequest) (*chief.SkillResponse, string, error) {
	skill, err := c.Skills.Create(ctx, &req)
	if err != nil {
		return nil, "", fmt.Errorf("create skill %q: %w", req.Name, err)
	}
	return skill, fmt.Sprintf("created skill %s (%s)", skill.Name, skill.SkillID), nil
}

func listSkills(ctx context.Context, c *chief.Client, req listSkillsRequest) (*chief.SkillPage, string, error) {
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

	page, err := c.Skills.List(ctx, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("list skills: %w", err)
	}
	return page, fmt.Sprintf("%d skill(s) returned (has_more %t)", len(page.Data), page.HasMore), nil
}

func getSkill(ctx context.Context, c *chief.Client, req skillIDRequest) (*chief.SkillResponse, string, error) {
	skill, err := c.Skills.Get(ctx, req.SkillID)
	if err != nil {
		return nil, "", fmt.Errorf("get skill %q: %w", req.SkillID, err)
	}
	return skill, fmt.Sprintf("skill %s: %s (enabled %t)", skill.SkillID, skill.Name, skill.Enabled), nil
}

func updateSkill(ctx context.Context, c *chief.Client, req updateSkillRequest) (*chief.SkillResponse, string, error) {
	skill, err := c.Skills.Update(ctx, req.SkillID, &req.Skill)
	if err != nil {
		return nil, "", fmt.Errorf("update skill %q: %w", req.SkillID, err)
	}
	return skill, fmt.Sprintf("updated skill %s (%s)", skill.Name, skill.SkillID), nil
}

func deleteSkill(ctx context.Context, c *chief.Client, req skillIDRequest) (deleteSkillResponse, string, error) {
	if err := c.Skills.Delete(ctx, req.SkillID); err != nil {
		return deleteSkillResponse{}, "", fmt.Errorf("delete skill %q: %w", req.SkillID, err)
	}
	return deleteSkillResponse{Deleted: true}, fmt.Sprintf("deleted skill %s", req.SkillID), nil
}

func enableSkill(ctx context.Context, c *chief.Client, req skillIDRequest) (*chief.SkillResponse, string, error) {
	skill, err := c.Skills.Enable(ctx, req.SkillID)
	if err != nil {
		return nil, "", fmt.Errorf("enable skill %q: %w", req.SkillID, err)
	}
	return skill, fmt.Sprintf("enabled skill %s", skill.SkillID), nil
}

func disableSkill(ctx context.Context, c *chief.Client, req skillIDRequest) (*chief.SkillResponse, string, error) {
	skill, err := c.Skills.Disable(ctx, req.SkillID)
	if err != nil {
		return nil, "", fmt.Errorf("disable skill %q: %w", req.SkillID, err)
	}
	return skill, fmt.Sprintf("disabled skill %s", skill.SkillID), nil
}

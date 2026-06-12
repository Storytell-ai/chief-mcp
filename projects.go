package main

import (
	"context"
	"fmt"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolListProjects            = "list_projects"
	toolCreateProject           = "create_project"
	toolUpdateProject           = "update_project"
	toolListProjectMembers      = "list_project_members"
	toolCreateProjectInvitation = "create_project_invitation"
	toolDeleteProjectInvitation = "delete_project_invitation"
)

type listProjectsRequest struct{}

type projectIDRequest struct {
	ProjectID string `json:"project_id" jsonschema:"the project ID"`
}

type updateProjectRequest struct {
	ProjectID string                     `json:"project_id" jsonschema:"the project to update"`
	Project   chief.UpdateProjectRequest `json:"project" jsonschema:"the full set of the two mutable fields, not a patch; an empty description clears it"`
}

type createProjectInvitationRequest struct {
	ProjectID  string                               `json:"project_id" jsonschema:"the project to invite into"`
	Invitation chief.CreateProjectInvitationRequest `json:"invitation" jsonschema:"email is required. role is one of \"collaborator\", \"reader\", \"owner\""`
}

type deleteProjectInvitationRequest struct {
	ProjectID    string `json:"project_id" jsonschema:"the project the invitation belongs to"`
	InvitationID string `json:"invitation_id" jsonschema:"the invitation to revoke"`
}

type deleteProjectInvitationResponse struct {
	Deleted bool `json:"deleted"`
}

func registerProjectTools(s *mcp.Server, c *chief.Client) {
	addTool(s, c, toolMeta{
		name: toolListProjects,
		desc: "List the projects the API key can access. Project tools need only the API key, not the configured project ID.",
	}, listProjects)
	addTool(s, c, toolMeta{
		name: toolCreateProject,
		desc: "Create a project. It lands in the org and workspace of the caller's root grant.",
	}, createProject)
	addTool(s, c, toolMeta{
		name: toolUpdateProject,
		desc: "Replace a project's name and description — the only two mutable fields. The body is a full set, not a patch: an empty description clears it.",
	}, updateProject)
	addTool(s, c, toolMeta{
		name: toolListProjectMembers,
		desc: "List every user holding a grant in a project, with their role and how they joined.",
	}, listProjectMembers)
	addTool(s, c, toolMeta{
		name: toolCreateProjectInvitation,
		desc: "Invite one user to a project by email. role is one of \"collaborator\", \"reader\", \"owner\". The invitation email is sent automatically; the user becomes a member only after accepting. A project holds one invitation per role: when one already exists the email is added to it, and re-inviting an email already on it succeeds unchanged.",
	}, createProjectInvitation)
	addTool(s, c, toolMeta{
		name: toolDeleteProjectInvitation,
		desc: "Revoke a pending project invitation by ID.",
	}, deleteProjectInvitation)
}

func listProjects(ctx context.Context, c *chief.Client, _ listProjectsRequest) (*chief.ProjectPage, string, error) {
	page, err := c.Projects.List(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("list projects: %w", err)
	}
	return page, fmt.Sprintf("%d project(s) returned (has_more %t)", len(page.Data), page.HasMore), nil
}

func createProject(ctx context.Context, c *chief.Client, req chief.CreateProjectRequest) (*chief.Project, string, error) {
	project, err := c.Projects.Create(ctx, &req)
	if err != nil {
		return nil, "", fmt.Errorf("create project %q: %w", req.Name, err)
	}
	return project, fmt.Sprintf("created project %s (%s)", project.Name, project.ProjectID), nil
}

func updateProject(ctx context.Context, c *chief.Client, req updateProjectRequest) (*chief.Project, string, error) {
	project, err := c.Projects.Update(ctx, req.ProjectID, &req.Project)
	if err != nil {
		return nil, "", fmt.Errorf("update project %q: %w", req.ProjectID, err)
	}
	return project, fmt.Sprintf("updated project %s (%s)", project.Name, project.ProjectID), nil
}

func listProjectMembers(ctx context.Context, c *chief.Client, req projectIDRequest) (*chief.ProjectMemberList, string, error) {
	list, err := c.Projects.ListMembers(ctx, req.ProjectID)
	if err != nil {
		return nil, "", fmt.Errorf("list members of project %q: %w", req.ProjectID, err)
	}
	return list, fmt.Sprintf("%d member(s) in project %s", len(list.Data), req.ProjectID), nil
}

func createProjectInvitation(ctx context.Context, c *chief.Client, req createProjectInvitationRequest) (*chief.ProjectInvitationResponse, string, error) {
	invitation, err := c.Projects.CreateInvitation(ctx, req.ProjectID, &req.Invitation)
	if err != nil {
		return nil, "", fmt.Errorf("invite %q to project %q: %w", req.Invitation.Email, req.ProjectID, err)
	}
	return invitation, fmt.Sprintf("invited %s to project %s as %s (invitation %s, invite link %s)", invitation.Email, req.ProjectID, invitation.Role, invitation.InvitationID, invitation.URL), nil
}

func deleteProjectInvitation(ctx context.Context, c *chief.Client, req deleteProjectInvitationRequest) (deleteProjectInvitationResponse, string, error) {
	if err := c.Projects.DeleteInvitation(ctx, req.ProjectID, req.InvitationID); err != nil {
		return deleteProjectInvitationResponse{}, "", fmt.Errorf("delete invitation %q from project %q: %w", req.InvitationID, req.ProjectID, err)
	}
	return deleteProjectInvitationResponse{Deleted: true}, fmt.Sprintf("deleted invitation %s from project %s", req.InvitationID, req.ProjectID), nil
}

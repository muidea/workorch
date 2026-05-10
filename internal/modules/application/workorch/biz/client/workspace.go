package client

import (
	"context"
	"fmt"
)

// ListWorkspaces returns all workspaces.
func (c *Client) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/workspaces", nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Workspace](resp, "workspaces")
}

// GetWorkspace returns a single workspace by ID.
func (c *Client) GetWorkspace(ctx context.Context, id string) (*Workspace, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Workspace](resp, "workspace")
}

// CreateWorkspace creates a new workspace.
func (c *Client) CreateWorkspace(ctx context.Context, title, description, workingDir string, templateID string) (*Workspace, error) {
	req := struct {
		Title            string `json:"title"`
		Description      string `json:"description,omitempty"`
		WorkingDirectory string `json:"workingDirectory,omitempty"`
		TemplateID       string `json:"templateId,omitempty"`
	}{Title: title, Description: description, WorkingDirectory: workingDir, TemplateID: templateID}
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/workspaces", req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Workspace](resp, "workspace")
}

// DeleteWorkspace deletes a workspace.
func (c *Client) DeleteWorkspace(ctx context.Context, id string, purge bool) error {
	path := fmt.Sprintf("/api/v1/workorch/workspaces/%s", id)
	if purge {
		path += "?purge=true"
	}
	resp, err := c.do(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetWorkspaceSnapshot returns the workspace snapshot (aggregated read model).
func (c *Client) GetWorkspaceSnapshot(ctx context.Context, id string) (*WorkspaceSnapshot, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/snapshot", id), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[WorkspaceSnapshot](resp, "snapshot")
}

// ListGoals returns goals for a workspace.
func (c *Client) ListGoals(ctx context.Context, workspaceID string) ([]Goal, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals", workspaceID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Goal](resp, "goals")
}

// CreateGoal defines a new goal for a workspace.
func (c *Client) CreateGoal(ctx context.Context, workspaceID, text string) (*Goal, error) {
	req := struct {
		Text string `json:"text"`
	}{Text: text}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals", workspaceID), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Goal](resp, "goal")
}

// GetGoal returns a single goal.
func (c *Client) GetGoal(ctx context.Context, goalID string) (*Goal, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/goals/%s", goalID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Goal](resp, "goal")
}

// ListDoDs returns DoDs for a workspace.
func (c *Client) ListDoDs(ctx context.Context, workspaceID string) ([]DoD, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods", workspaceID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[DoD](resp, "dods")
}

// CreateDoD defines a new DoD for a workspace.
func (c *Client) CreateDoD(ctx context.Context, workspaceID string, items []string) (*DoD, error) {
	req := struct {
		Items []string `json:"items"`
	}{Items: items}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods", workspaceID), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[DoD](resp, "dod")
}

// GetDoD returns a single DoD.
func (c *Client) GetDoD(ctx context.Context, dodID string) (*DoD, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/dods/%s", dodID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[DoD](resp, "dod")
}

// ActivateWorkspaceExecution activates a workspace execution (create + start run).
func (c *Client) ActivateWorkspaceExecution(ctx context.Context, workspaceID string, req *ActivationRequest) (*Activation, error) {
	path := fmt.Sprintf("/api/v1/workorch/workspaces/%s/execution-activations", workspaceID)
	resp, err := c.do(ctx, "POST", path, req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Activation](resp, "activation")
}

// ActivationRequest is the request body for activating workspace execution.
type ActivationRequest struct {
	GoalID                string            `json:"goalId,omitempty"`
	DoDID                 string            `json:"doDId,omitempty"`
	LLMProviderID         string            `json:"llmProviderId,omitempty"`
	SoulID                string            `json:"soulId,omitempty"`
	SoulBindings          map[string]string `json:"soulBindings,omitempty"`
	SkillIDs              []string          `json:"skillIds,omitempty"`
	MCPServerIDs          []string          `json:"mcpServerIds,omitempty"`
	CapabilityIDs         []string          `json:"capabilityIds,omitempty"`
	MemoryIDs             []string          `json:"memoryIds,omitempty"`
	DialogLanguage        string            `json:"dialogLanguage,omitempty"`
	Priority              int               `json:"priority,omitempty"`
	ExecutionContextPatch map[string]any    `json:"executionContextPatch,omitempty"`
}

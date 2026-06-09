package workorchclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ListWorkspaces returns all workspaces.
func (c *Client) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	projection, err := c.ListWorkspaceProjection(ctx)
	if err != nil {
		return nil, err
	}
	return projection.Workspaces, nil
}

// ListWorkspaceProjection returns workspace overviews with server projection metadata.
func (c *Client) ListWorkspaceProjection(ctx context.Context) (*WorkspaceListProjection, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/workorch/workspaces", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result WorkspaceListProjection
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode workspace projection envelope: %w", err)
	}
	return &result, nil
}

// GetWorkspace returns a single workspace by ID.
func (c *Client) GetWorkspace(ctx context.Context, id string) (*Workspace, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s", escapePath(id)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Workspace](resp, "workspace")
}

// CreateWorkspace creates a new workspace.
func (c *Client) CreateWorkspace(ctx context.Context, title, description, workingDir string, templateID string) (*Workspace, error) {
	req := struct {
		Title                string `json:"title"`
		Description          string `json:"description,omitempty"`
		DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
		TemplateID           string `json:"templateId,omitempty"`
	}{Title: title, Description: description, DeliverableDirectory: workingDir, TemplateID: templateID}
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/workspaces", req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Workspace](resp, "workspace")
}

// DeleteWorkspace deletes a workspace.
func (c *Client) DeleteWorkspace(ctx context.Context, id string, purge bool) error {
	values := url.Values{}
	if purge {
		values.Set("purge", "true")
	}
	path := joinQuery(fmt.Sprintf("/api/v1/workorch/workspaces/%s", escapePath(id)), values)
	resp, err := c.do(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetWorkspaceSnapshot returns the workspace snapshot (aggregated read model).
func (c *Client) GetWorkspaceSnapshot(ctx context.Context, id string) (*WorkspaceSnapshot, error) {
	projection, err := c.GetWorkspaceSnapshotProjection(ctx, id)
	if err != nil {
		return nil, err
	}
	return &projection.Snapshot, nil
}

// GetWorkspaceSnapshotProjection returns workspace detail with server projection metadata.
func (c *Client) GetWorkspaceSnapshotProjection(ctx context.Context, id string) (*WorkspaceSnapshotProjection, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/snapshot", escapePath(id)), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result WorkspaceSnapshotProjection
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode workspace snapshot projection envelope: %w", err)
	}
	return &result, nil
}

// ListGoals returns goals for a workspace.
func (c *Client) ListGoals(ctx context.Context, workspaceID string) ([]Goal, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals", escapePath(workspaceID)), nil)
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
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals", escapePath(workspaceID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Goal](resp, "goal")
}

// GetGoal returns a single goal scoped to a workspace.
func (c *Client) GetGoal(ctx context.Context, workspaceID string, goalID string) (*Goal, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals/%s", escapePath(workspaceID), escapePath(goalID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Goal](resp, "goal")
}

// UpdateGoal replaces an existing goal text scoped to a workspace.
func (c *Client) UpdateGoal(ctx context.Context, workspaceID string, goalID string, text string) (*Goal, error) {
	req := struct {
		Text string `json:"text"`
	}{Text: text}
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals/%s", escapePath(workspaceID), escapePath(goalID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Goal](resp, "goal")
}

// ListDoDs returns DoDs for a workspace.
func (c *Client) ListDoDs(ctx context.Context, workspaceID string) ([]DoD, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods", escapePath(workspaceID)), nil)
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
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods", escapePath(workspaceID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[DoD](resp, "dod")
}

// GetDoD returns a single DoD scoped to a workspace.
func (c *Client) GetDoD(ctx context.Context, workspaceID string, dodID string) (*DoD, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods/%s", escapePath(workspaceID), escapePath(dodID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[DoD](resp, "dod")
}

// UpdateDoD replaces the items of an existing DoD scoped to a workspace.
func (c *Client) UpdateDoD(ctx context.Context, workspaceID string, dodID string, items []string) (*DoD, error) {
	req := struct {
		Items []string `json:"items"`
	}{Items: items}
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods/%s", escapePath(workspaceID), escapePath(dodID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[DoD](resp, "dod")
}

// ActivateWorkspaceExecution activates a workspace execution (create + start run).
func (c *Client) ActivateWorkspaceExecution(ctx context.Context, workspaceID string, req *ActivationRequest) (*Activation, error) {
	path := fmt.Sprintf("/api/v1/workorch/workspaces/%s/execution-activations", escapePath(workspaceID))
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

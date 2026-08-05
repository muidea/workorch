package workorchclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ListWorkspaces returns all workspaces.
func (c *Client) ListWorkspaces(ctx context.Context) ([]workorchprojectioncontract.Workspace, error) {
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
func (c *Client) GetWorkspace(ctx context.Context, id string) (*workorchprojectioncontract.Workspace, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s", escapePath(id)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchprojectioncontract.Workspace](resp, "workspace")
}

// CreateWorkspace creates a workspace from its selected template and ordinary
// user metadata. Runtime contract declarations are template-owned.
func (c *Client) CreateWorkspace(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*workorchprojectioncontract.Workspace, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/workspaces", req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchprojectioncontract.Workspace](resp, "workspace")
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

func (c *Client) UpdateWorkspaceLLMTarget(ctx context.Context, id string, target runtimecontract.ProviderModelTarget) (*workorchprojectioncontract.Workspace, error) {
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/llm-target", escapePath(id)), target)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchprojectioncontract.Workspace](resp, "workspace")
}

func (c *Client) ClearWorkspaceLLMTarget(ctx context.Context, id string) (*workorchprojectioncontract.Workspace, error) {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/workorch/workspaces/%s/llm-target", escapePath(id)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchprojectioncontract.Workspace](resp, "workspace")
}

// GetWorkspaceSnapshot returns the workspace snapshot (aggregated read model).
func (c *Client) GetWorkspaceSnapshot(ctx context.Context, id string) (*workorchprojectioncontract.WorkspaceSnapshot, error) {
	projection, err := c.GetWorkspaceSnapshotProjection(ctx, id)
	if err != nil {
		return nil, err
	}
	return &projection.Snapshot, nil
}

// GetWorkspaceSnapshotProjection returns workspace detail with server projection metadata.
func (c *Client) GetWorkspaceSnapshotProjection(ctx context.Context, id string) (*WorkspaceSnapshotProjection, error) {
	values := url.Values{}
	values.Set("view", "detail")
	resp, err := c.do(ctx, "GET", joinQuery(fmt.Sprintf("/api/v1/workorch/workspaces/%s/snapshot", escapePath(id)), values), nil)
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

// GetWorkspaceRuntimeView returns the lightweight runtime main-view projection.
func (c *Client) GetWorkspaceRuntimeView(ctx context.Context, id string, activityLimit int, beforeCursor string) (*WorkspaceRuntimeViewProjection, error) {
	values := url.Values{}
	if activityLimit > 0 {
		values.Set("activityLimit", fmt.Sprintf("%d", activityLimit))
	}
	if strings.TrimSpace(beforeCursor) != "" {
		values.Set("beforeCursor", beforeCursor)
	}
	resp, err := c.do(ctx, "GET", joinQuery(fmt.Sprintf("/api/v1/workorch/workspaces/%s/runtime-view", escapePath(id)), values), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result WorkspaceRuntimeViewProjection
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode workspace runtime view envelope: %w", err)
	}
	if strings.TrimSpace(result.Error) != "" {
		return nil, fmt.Errorf("workspace runtime view: %s", result.Error)
	}
	return &result, nil
}

// ListGoals returns goals for a workspace.
func (c *Client) ListGoals(ctx context.Context, workspaceID string) ([]workorchmanagementcontract.Goal, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals", escapePath(workspaceID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchmanagementcontract.Goal](resp, "goals")
}

// CreateGoal defines a new goal for a workspace.
func (c *Client) CreateGoal(ctx context.Context, workspaceID, text string) (*workorchmanagementcontract.Goal, error) {
	req := struct {
		Text string `json:"text"`
	}{Text: text}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals", escapePath(workspaceID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.Goal](resp, "goal")
}

// GetGoal returns a single goal scoped to a workspace.
func (c *Client) GetGoal(ctx context.Context, workspaceID string, goalID string) (*workorchmanagementcontract.Goal, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals/%s", escapePath(workspaceID), escapePath(goalID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.Goal](resp, "goal")
}

// UpdateGoal replaces an existing goal text scoped to a workspace.
func (c *Client) UpdateGoal(ctx context.Context, workspaceID string, goalID string, text string) (*workorchmanagementcontract.Goal, error) {
	req := struct {
		Text string `json:"text"`
	}{Text: text}
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals/%s", escapePath(workspaceID), escapePath(goalID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.Goal](resp, "goal")
}

// ListDoDs returns DoDs for a workspace.
func (c *Client) ListDoDs(ctx context.Context, workspaceID string) ([]workorchmanagementcontract.DoD, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods", escapePath(workspaceID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchmanagementcontract.DoD](resp, "dods")
}

// CreateDoD defines a new workorchmanagementcontract.DoD for a workspace.
func (c *Client) CreateDoD(ctx context.Context, workspaceID string, items []string) (*workorchmanagementcontract.DoD, error) {
	req := struct {
		Items []string `json:"items"`
	}{Items: items}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods", escapePath(workspaceID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.DoD](resp, "dod")
}

// GetDoD returns a single workorchmanagementcontract.DoD scoped to a workspace.
func (c *Client) GetDoD(ctx context.Context, workspaceID string, dodID string) (*workorchmanagementcontract.DoD, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods/%s", escapePath(workspaceID), escapePath(dodID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.DoD](resp, "dod")
}

// UpdateDoD replaces the items of an existing workorchmanagementcontract.DoD scoped to a workspace.
func (c *Client) UpdateDoD(ctx context.Context, workspaceID string, dodID string, items []string) (*workorchmanagementcontract.DoD, error) {
	req := struct {
		Items []string `json:"items"`
	}{Items: items}
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods/%s", escapePath(workspaceID), escapePath(dodID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.DoD](resp, "dod")
}

// ResumeWorkspaceExecution submits the workspace-scoped resume command.
//
// The REST resource path is still named execution-activations because it
// returns the execution activation contract used to create or resume the run.
// Client callers should treat this as command acceptance, not as a local
// activation state machine.
func (c *Client) ResumeWorkspaceExecution(ctx context.Context, workspaceID string, req *workorchmanagementcontract.WorkspaceExecutionCommandRequest) (*workorchprojectioncontract.WorkspaceExecutionActivation, error) {
	path := fmt.Sprintf("/api/v1/workorch/workspaces/%s/execution-activations", escapePath(workspaceID))
	resp, err := c.do(ctx, "POST", path, req)
	if err != nil {
		return nil, err
	}
	if accepted, err := drainAccepted(resp); accepted || err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchprojectioncontract.WorkspaceExecutionActivation](resp, "activation")
}

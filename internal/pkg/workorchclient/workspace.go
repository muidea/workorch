package workorchclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	executioncontract "github.com/muidea/workorch/internal/pkg/workorchexecutioncontract"
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

func (c *Client) UpdateWorkspaceLLMSelection(ctx context.Context, id string, selection runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (*workorchprojectioncontract.Workspace, error) {
	selection = selection.Normalized()
	resp, err := c.do(ctx, "PUT", fmt.Sprintf("/api/v1/workorch/workspaces/%s/llm-selection", escapePath(id)), struct {
		ProviderID       string `json:"providerId"`
		ModelID          string `json:"modelId"`
		ReasoningEffort  string `json:"reasoningEffort,omitempty"`
		ExpectedRevision uint64 `json:"expectedRevision"`
	}{ProviderID: selection.Target.ProviderID, ModelID: selection.Target.ModelID, ReasoningEffort: selection.ReasoningEffort, ExpectedRevision: expectedRevision})
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchprojectioncontract.Workspace](resp, "workspace")
}

func (c *Client) ClearWorkspaceLLMSelection(ctx context.Context, id string, expectedRevision uint64) (*workorchprojectioncontract.Workspace, error) {
	resp, err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/workorch/workspaces/%s/llm-selection", escapePath(id)), struct {
		ExpectedRevision uint64 `json:"expectedRevision"`
	}{ExpectedRevision: expectedRevision})
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

// GetGoal returns a single goal scoped to a workspace.
func (c *Client) GetGoal(ctx context.Context, workspaceID string, goalID string) (*workorchmanagementcontract.Goal, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/goals/%s", escapePath(workspaceID), escapePath(goalID)), nil)
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

// GetDoD returns a single workorchmanagementcontract.DoD scoped to a workspace.
func (c *Client) GetDoD(ctx context.Context, workspaceID string, dodID string) (*workorchmanagementcontract.DoD, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/workspaces/%s/dods/%s", escapePath(workspaceID), escapePath(dodID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchmanagementcontract.DoD](resp, "dod")
}

// ResumeWorkspaceExecution returns the exact synchronous Workspace-owner
// admission for either HTTP 202 acceptance or HTTP 409 owner rejection.
func (c *Client) ResumeWorkspaceExecution(ctx context.Context, workspaceID string, req *workorchmanagementcontract.WorkspaceExecutionCommandRequest) (*runtimecontract.RuntimeExecutionIntentAdmission, error) {
	path := fmt.Sprintf("/api/v1/workorch/workspaces/%s/execution-activations", escapePath(workspaceID))
	resp, err := c.send(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusConflict {
		return nil, apiErrorFromResponse(resp)
	}
	defer resp.Body.Close()
	var response executioncontract.WorkspaceExecutionAdmissionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode workspace execution admission: %w", err)
	}
	admission, err := response.Admission()
	if err != nil {
		return nil, fmt.Errorf("%w: decode workspace execution admission: %v", common.ErrConflict, err)
	}
	if string(admission.WorkspaceID) != strings.TrimSpace(workspaceID) {
		return nil, fmt.Errorf("%w: workspace execution admission identity %q does not match request workspace %q", common.ErrConflict, admission.WorkspaceID, strings.TrimSpace(workspaceID))
	}
	if req != nil && strings.TrimSpace(req.RequestID) != "" && admission.RequestID != strings.TrimSpace(req.RequestID) {
		return nil, fmt.Errorf("%w: workspace execution admission request identity %q does not match request %q", common.ErrConflict, admission.RequestID, strings.TrimSpace(req.RequestID))
	}
	if req != nil {
		intent := strings.TrimSpace(req.Intent)
		if intent == "" {
			intent = string(runtimecontract.RuntimeExecutionIntentResume)
		}
		if string(admission.Kind) != intent {
			return nil, fmt.Errorf("%w: workspace execution admission intent %q does not match request %q", common.ErrConflict, admission.Kind, intent)
		}
	}
	wantStatus := http.StatusConflict
	if response.Accepted {
		wantStatus = http.StatusAccepted
	}
	if resp.StatusCode != wantStatus {
		return nil, fmt.Errorf("%w: workspace execution admission HTTP status %d does not match typed outcome", common.ErrConflict, resp.StatusCode)
	}
	return &admission, nil
}

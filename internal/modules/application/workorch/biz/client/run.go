package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/muidea/workorch/internal/pkg/common"
)

// ListRuns returns all runs, optionally filtered by workspace.
func (c *Client) ListRuns(ctx context.Context, workspaceID string) ([]Run, error) {
	path := "/api/v1/workorch/runs"
	if workspaceID != "" {
		path += "?workspaceId=" + workspaceID
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Run](resp, "runs")
}

// GetRun returns a single run.
func (c *Client) GetRun(ctx context.Context, runID string) (*Run, error) {
	resp, err := c.do(ctx, "GET", runPath(ctx, runID, ""), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Run](resp, "run")
}

// CreateRun creates a new run for a workspace.
func (c *Client) CreateRun(ctx context.Context, workspaceID, runID, goalID, dodID string) (*Run, error) {
	req := struct {
		RunID  string `json:"runId"`
		GoalID string `json:"goalId,omitempty"`
		DoDID  string `json:"doDId,omitempty"`
	}{RunID: runID, GoalID: goalID, DoDID: dodID}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/workspaces/%s/runs", workspaceID), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Run](resp, "run")
}

// StartRun starts a created run.
func (c *Client) StartRun(ctx context.Context, runID string) (*Run, error) {
	resp, err := c.do(ctx, "POST", runPath(ctx, runID, "start"), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Run](resp, "run")
}

// ResumeRun resumes a blocked run, optionally merging additional execution context.
func (c *Client) ResumeRun(ctx context.Context, runID string, executionContextPatch ...map[string]any) (*Run, error) {
	var body any
	if len(executionContextPatch) > 0 && len(executionContextPatch[0]) > 0 {
		body = struct {
			ExecutionContextPatch map[string]any `json:"executionContextPatch,omitempty"`
		}{ExecutionContextPatch: executionContextPatch[0]}
	}
	resp, err := c.do(ctx, "POST", runPath(ctx, runID, "resume"), body)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Run](resp, "run")
}

// GetRunStatus returns the status of a run, optionally with long-poll waiting.
func (c *Client) GetRunStatus(ctx context.Context, runID, waitState string, waitTimeoutMs int) (*RunStatus, error) {
	path := runPath(ctx, runID, "status")
	if waitState != "" {
		path += fmt.Sprintf("?waitState=%s", url.QueryEscape(waitState))
		if waitTimeoutMs > 0 {
			path += fmt.Sprintf("&waitTimeoutMillis=%d", waitTimeoutMs)
		}
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[RunStatus](resp, "status")
}

// GetRunSnapshot returns the full run snapshot.
func (c *Client) GetRunSnapshot(ctx context.Context, runID string) (*RunSnapshot, error) {
	resp, err := c.do(ctx, "GET", runPath(ctx, runID, "snapshot"), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[RunSnapshot](resp, "snapshot")
}

// GetRunObservability returns the run observability with timeline.
func (c *Client) GetRunObservability(ctx context.Context, runID string) (*RunObservability, error) {
	resp, err := c.do(ctx, "GET", runPath(ctx, runID, "observability"), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[RunObservability](resp, "observability")
}

func runPath(ctx context.Context, runID string, action string) string {
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	encodedRunID := url.PathEscape(runID)
	if workspaceID == "" {
		if action == "" {
			return fmt.Sprintf("/api/v1/workorch/runs/%s", encodedRunID)
		}
		return fmt.Sprintf("/api/v1/workorch/runs/%s/%s", encodedRunID, action)
	}
	if action == "" {
		return fmt.Sprintf("/api/v1/workorch/workspaces/%s/runs/%s", url.PathEscape(string(workspaceID)), encodedRunID)
	}
	return fmt.Sprintf("/api/v1/workorch/workspaces/%s/runs/%s/%s", url.PathEscape(string(workspaceID)), encodedRunID, action)
}

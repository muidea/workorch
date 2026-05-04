package client

import (
	"context"
	"fmt"
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
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/runs/%s", runID), nil)
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
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/runs/%s/start", runID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Run](resp, "run")
}

// ResumeRun resumes a blocked run.
func (c *Client) ResumeRun(ctx context.Context, runID string) (*Run, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/runs/%s/resume", runID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Run](resp, "run")
}

// GetRunStatus returns the status of a run, optionally with long-poll waiting.
func (c *Client) GetRunStatus(ctx context.Context, runID, waitState string, waitTimeoutMs int) (*RunStatus, error) {
	path := fmt.Sprintf("/api/v1/workorch/runs/%s/status", runID)
	if waitState != "" {
		path += fmt.Sprintf("?waitState=%s", waitState)
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
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/runs/%s/snapshot", runID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[RunSnapshot](resp, "snapshot")
}

// GetRunObservability returns the run observability with timeline.
func (c *Client) GetRunObservability(ctx context.Context, runID string) (*RunObservability, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/runs/%s/observability", runID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[RunObservability](resp, "observability")
}

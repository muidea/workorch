package workorchclient

import (
	"context"
	"fmt"
	"net/url"
)

// ListRuns returns all runs, optionally filtered by workspace.
func (c *Client) ListRuns(ctx context.Context, workspaceID string) ([]Run, error) {
	path := "/api/v1/workorch/runs"
	if workspaceID != "" {
		values := url.Values{}
		values.Set("workspaceId", workspaceID)
		path = joinQuery(path, values)
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Run](resp, "runs")
}

// GetRun returns a single run scoped to a workspace.
func (c *Client) GetRun(ctx context.Context, workspaceID, runID string) (*Run, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	resp, err := c.do(ctx, "GET", runPath(workspaceID, runID, ""), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Run](resp, "run")
}

// GetRunStatus returns the status of a run, optionally with long-poll waiting.
func (c *Client) GetRunStatus(ctx context.Context, workspaceID, runID, waitState string, waitTimeoutMs int) (*RunStatus, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	return c.getRunStatus(ctx, runPath(workspaceID, runID, "status"), waitState, waitTimeoutMs)
}

func (c *Client) getRunStatus(ctx context.Context, path string, waitState string, waitTimeoutMs int) (*RunStatus, error) {
	values := url.Values{}
	if waitState != "" {
		values.Set("waitState", waitState)
		if waitTimeoutMs > 0 {
			values.Set("waitTimeoutMillis", fmt.Sprintf("%d", waitTimeoutMs))
		}
	}
	path = joinQuery(path, values)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[RunStatus](resp, "status")
}

// GetRunSnapshot returns the full run snapshot.
func (c *Client) GetRunSnapshot(ctx context.Context, workspaceID, runID string) (*RunSnapshot, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	resp, err := c.do(ctx, "GET", runPath(workspaceID, runID, "snapshot"), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[RunSnapshot](resp, "snapshot")
}

// GetRunObservability returns the run observability with timeline.
func (c *Client) GetRunObservability(ctx context.Context, workspaceID, runID string) (*RunObservability, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	resp, err := c.do(ctx, "GET", runPath(workspaceID, runID, "observability"), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[RunObservability](resp, "observability")
}

func runPath(workspaceID string, runID string, action string) string {
	encodedRunID := escapePath(runID)
	if action == "" {
		return fmt.Sprintf("/api/v1/workorch/workspaces/%s/runs/%s", escapePath(workspaceID), encodedRunID)
	}
	return fmt.Sprintf("/api/v1/workorch/workspaces/%s/runs/%s/%s", escapePath(workspaceID), encodedRunID, action)
}

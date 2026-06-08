package client

import (
	"context"
	"fmt"
	"net/url"
)

// ListApprovals returns all approvals, optionally filtered.
func (c *Client) ListApprovals(ctx context.Context, workspaceID, runID string) ([]Approval, error) {
	path := "/api/v1/workorch/approvals"
	values := url.Values{}
	if workspaceID != "" {
		values.Set("workspaceId", workspaceID)
	}
	if runID != "" {
		values.Set("runId", runID)
	}
	path = joinQuery(path, values)
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Approval](resp, "approvals")
}

// GetApproval returns a single approval.
func (c *Client) GetApproval(ctx context.Context, approvalID string) (*Approval, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/approvals/%s", escapePath(approvalID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Approval](resp, "approval")
}

// Approve approves an approval gate.
func (c *Client) Approve(ctx context.Context, approvalID string) (*Approval, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/approvals/%s/approve", escapePath(approvalID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Approval](resp, "approval")
}

// Reject rejects an approval gate.
func (c *Client) Reject(ctx context.Context, approvalID string) (*Approval, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/approvals/%s/reject", escapePath(approvalID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Approval](resp, "approval")
}

// BatchResolveApprovals resolves multiple approvals at once.
func (c *Client) BatchResolveApprovals(ctx context.Context, approvalIDs []string, action string) ([]Approval, error) {
	req := struct {
		ApprovalIDs []string `json:"approvalIds"`
		Action      string   `json:"action"`
	}{ApprovalIDs: approvalIDs, Action: action}
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/approvals/resolve-batch", req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Approval](resp, "approvals")
}

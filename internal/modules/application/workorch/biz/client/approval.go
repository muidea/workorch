package client

import (
	"context"
	"fmt"
)

// ListApprovals returns all approvals, optionally filtered.
func (c *Client) ListApprovals(ctx context.Context, workspaceID, runID string) ([]Approval, error) {
	path := "/api/v1/workorch/approvals"
	parts := []string{}
	if workspaceID != "" {
		parts = append(parts, "workspaceId="+workspaceID)
	}
	if runID != "" {
		parts = append(parts, "runId="+runID)
	}
	if len(parts) > 0 {
		path += "?"
		for i, p := range parts {
			if i > 0 {
				path += "&"
			}
			path += p
		}
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Approval](resp, "approvals")
}

// GetApproval returns a single approval.
func (c *Client) GetApproval(ctx context.Context, approvalID string) (*Approval, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/approvals/%s", approvalID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Approval](resp, "approval")
}

// Approve approves an approval gate.
func (c *Client) Approve(ctx context.Context, approvalID string) (*Approval, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/approvals/%s/approve", approvalID), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Approval](resp, "approval")
}

// Reject rejects an approval gate.
func (c *Client) Reject(ctx context.Context, approvalID string) (*Approval, error) {
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/approvals/%s/reject", approvalID), nil)
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

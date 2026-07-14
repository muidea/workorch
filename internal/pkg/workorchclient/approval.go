package workorchclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
func (c *Client) GetApproval(ctx context.Context, workspaceID string, approvalID string) (*Approval, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	path := fmt.Sprintf("/api/v1/workorch/approvals/%s", escapePath(approvalID))
	values := url.Values{}
	if workspaceID != "" {
		values.Set("workspaceId", workspaceID)
	}
	resp, err := c.do(ctx, "GET", joinQuery(path, values), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Approval](resp, "approval")
}

// Approve approves an approval gate.
func (c *Client) Approve(ctx context.Context, workspaceID string, approvalID string, payload ...workorcheventpayload.Payload) (*Approval, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	path := fmt.Sprintf("/api/v1/workorch/approvals/%s/approve", escapePath(approvalID))
	values := url.Values{}
	if workspaceID != "" {
		values.Set("workspaceId", workspaceID)
	}
	resp, err := c.do(ctx, "POST", joinQuery(path, values), approvalResolveRequestPayload(firstApprovalPayload(payload)))
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Approval](resp, "approval")
}

// Reject rejects an approval gate.
func (c *Client) Reject(ctx context.Context, workspaceID string, approvalID string, payload ...workorcheventpayload.Payload) (*Approval, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	path := fmt.Sprintf("/api/v1/workorch/approvals/%s/reject", escapePath(approvalID))
	values := url.Values{}
	if workspaceID != "" {
		values.Set("workspaceId", workspaceID)
	}
	resp, err := c.do(ctx, "POST", joinQuery(path, values), approvalResolveRequestPayload(firstApprovalPayload(payload)))
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Approval](resp, "approval")
}

// BatchResolveApprovals resolves multiple approvals at once.
func (c *Client) BatchResolveApprovals(ctx context.Context, workspaceID string, approvalIDs []string, action string, payload ...workorcheventpayload.Payload) ([]Approval, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	req := struct {
		ApprovalIDs []string                     `json:"approvalIds"`
		Action      string                       `json:"action"`
		Payload     workorcheventpayload.Payload `json:"payload,omitempty"`
	}{ApprovalIDs: approvalIDs, Action: action, Payload: firstApprovalPayload(payload)}
	values := url.Values{}
	if workspaceID != "" {
		values.Set("workspaceId", workspaceID)
	}
	resp, err := c.do(ctx, "POST", joinQuery("/api/v1/workorch/approvals/resolve-batch", values), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Approval](resp, "approvals")
}

func approvalResolveRequestPayload(payload workorcheventpayload.Payload) any {
	if len(payload.Fields) == 0 {
		return nil
	}
	return struct {
		Payload workorcheventpayload.Payload `json:"payload,omitempty"`
	}{Payload: payload}
}

func firstApprovalPayload(payload []workorcheventpayload.Payload) workorcheventpayload.Payload {
	for _, candidate := range payload {
		if len(candidate.Fields) > 0 {
			return candidate.Clone()
		}
	}
	return workorcheventpayload.Payload{}
}

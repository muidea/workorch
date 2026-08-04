package workorchclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// ListDecisions returns all decisions, optionally filtered.
func (c *Client) ListDecisions(ctx context.Context, workspaceID, runID string) ([]workorchprojectioncontract.Decision, error) {
	path := "/api/v1/workorch/decisions"
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
	return decodeEnvelopeSlice[workorchprojectioncontract.Decision](resp, "decisions")
}

// GetDecision returns a single decision ticket.
func (c *Client) GetDecision(ctx context.Context, workspaceID string, ticketID string) (*workorchprojectioncontract.Decision, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	path := fmt.Sprintf("/api/v1/workorch/decisions/%s", escapePath(ticketID))
	values := url.Values{}
	if workspaceID != "" {
		values.Set("workspaceId", workspaceID)
	}
	resp, err := c.do(ctx, "GET", joinQuery(path, values), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchprojectioncontract.Decision](resp, "decision")
}

// ResolveDecision resolves a decision ticket.
func (c *Client) ResolveDecision(ctx context.Context, workspaceID, ticketID, resolutionType string, payload workorcheventpayload.Payload) (*workorchprojectioncontract.Decision, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	req := struct {
		ResolutionType    string                       `json:"resolutionType"`
		ResolutionPayload workorcheventpayload.Payload `json:"resolutionPayload,omitempty"`
	}{ResolutionType: resolutionType, ResolutionPayload: payload}
	path := fmt.Sprintf("/api/v1/workorch/decisions/%s/resolve", escapePath(ticketID))
	values := url.Values{}
	if workspaceID != "" {
		values.Set("workspaceId", workspaceID)
	}
	resp, err := c.do(ctx, "POST", joinQuery(path, values), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[workorchprojectioncontract.Decision](resp, "decision")
}

// BatchResolveDecisions resolves multiple decision tickets with the same type.
func (c *Client) BatchResolveDecisions(ctx context.Context, workspaceID string, ticketIDs []string, resolutionType string, payload workorcheventpayload.Payload) ([]workorchprojectioncontract.Decision, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspaceId is required")
	}
	req := struct {
		TicketIDs         []string                     `json:"ticketIds"`
		ResolutionType    string                       `json:"resolutionType"`
		ResolutionPayload workorcheventpayload.Payload `json:"resolutionPayload,omitempty"`
	}{TicketIDs: ticketIDs, ResolutionType: resolutionType, ResolutionPayload: payload}
	values := url.Values{}
	if workspaceID != "" {
		values.Set("workspaceId", workspaceID)
	}
	resp, err := c.do(ctx, "POST", joinQuery("/api/v1/workorch/decisions/resolve-batch", values), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[workorchprojectioncontract.Decision](resp, "decisions")
}

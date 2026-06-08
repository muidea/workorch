package client

import (
	"context"
	"fmt"
	"net/url"
)

// ListDecisions returns all decisions, optionally filtered.
func (c *Client) ListDecisions(ctx context.Context, workspaceID, runID string) ([]Decision, error) {
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
	return decodeEnvelopeSlice[Decision](resp, "decisions")
}

// GetDecision returns a single decision ticket.
func (c *Client) GetDecision(ctx context.Context, ticketID string) (*Decision, error) {
	resp, err := c.do(ctx, "GET", fmt.Sprintf("/api/v1/workorch/decisions/%s", escapePath(ticketID)), nil)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Decision](resp, "decision")
}

// ResolveDecision resolves a decision ticket.
func (c *Client) ResolveDecision(ctx context.Context, ticketID, resolutionType string, payload map[string]any) (*Decision, error) {
	req := struct {
		ResolutionType    string         `json:"resolutionType"`
		ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
	}{ResolutionType: resolutionType, ResolutionPayload: payload}
	resp, err := c.do(ctx, "POST", fmt.Sprintf("/api/v1/workorch/decisions/%s/resolve", escapePath(ticketID)), req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelope[Decision](resp, "decision")
}

// BatchResolveDecisions resolves multiple decision tickets with the same type.
func (c *Client) BatchResolveDecisions(ctx context.Context, ticketIDs []string, resolutionType string, payload map[string]any) ([]Decision, error) {
	req := struct {
		TicketIDs         []string       `json:"ticketIds"`
		ResolutionType    string         `json:"resolutionType"`
		ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
	}{TicketIDs: ticketIDs, ResolutionType: resolutionType, ResolutionPayload: payload}
	resp, err := c.do(ctx, "POST", "/api/v1/workorch/decisions/resolve-batch", req)
	if err != nil {
		return nil, err
	}
	return decodeEnvelopeSlice[Decision](resp, "decisions")
}

package appmodelcore

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type DecisionQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type DecisionPage struct {
	WorkspaceID       string                        `json:"workspaceId,omitempty"`
	RunID             string                        `json:"runId,omitempty"`
	Decisions         []projectioncontract.Decision `json:"decisions,omitempty"`
	Limit             int                           `json:"limit"`
	CursorAfterID     string                        `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string                        `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                          `json:"hasMore"`
}

type ResolveDecisionCommand struct {
	ResolutionType    string                       `json:"resolutionType"`
	ResolutionPayload workorcheventpayload.Payload `json:"resolutionPayload,omitempty"`
}

type BatchResolveDecisionCommand struct {
	TicketIDs         []string                     `json:"ticketIds"`
	ResolutionType    string                       `json:"resolutionType"`
	ResolutionPayload workorcheventpayload.Payload `json:"resolutionPayload,omitempty"`
}

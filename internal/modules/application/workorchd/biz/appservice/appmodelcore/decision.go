package appmodelcore

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type DecisionPage struct {
	WorkspaceID string                        `json:"workspaceId,omitempty"`
	RunID       string                        `json:"runId,omitempty"`
	Decisions   []projectioncontract.Decision `json:"decisions,omitempty"`
	projectioncontract.CursorPage
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

package contracts

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type CreateTicketCommand struct {
	TicketID               common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	Reason                 string
	DecisionKind           string
	Question               string
	Context                workorcheventpayload.Payload
	AutoGovernance         workorcheventpayload.Payload
	FailureClass           string
	FailureSignature       string
	AllowedRecoveryActions []string
}

type UpdateTicketCommand struct {
	TicketID               common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	Reason                 string
	DecisionKind           string
	Question               string
	Context                workorcheventpayload.Payload
	State                  common.DecisionTicketState
	AutoGovernance         workorcheventpayload.Payload
	FailureClass           string
	FailureSignature       string
	AllowedRecoveryActions []string
	ResolvedAt             string
	ResolvedBy             string
	CreatedAt              string
	UpdatedAt              string
}

type ResolveTicketCommand struct {
	ResolutionID       common.ResolutionID
	TicketID           common.TicketID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	Type               string
	Payload            workorcheventpayload.Payload
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

type CreateResolutionCommand struct {
	ResolutionID       common.ResolutionID
	TicketID           common.TicketID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	Type               string
	Payload            workorcheventpayload.Payload
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

type DeleteTicketCommand struct {
	TicketID    common.TicketID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

type DeleteResolutionCommand struct {
	TicketID    common.TicketID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

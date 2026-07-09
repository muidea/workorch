package contracts

import "github.com/muidea/workorch/internal/pkg/common"

type CreateTicketCommand struct {
	TicketID               common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	Reason                 string
	DecisionKind           string
	Question               string
	Context                map[string]any
	AutoGovernance         map[string]any
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
	Context                map[string]any
	State                  common.DecisionTicketState
	AutoGovernance         map[string]any
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
	Payload            map[string]any
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
	Payload            map[string]any
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

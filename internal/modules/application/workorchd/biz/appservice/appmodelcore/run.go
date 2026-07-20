package appmodelcore

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type Run struct {
	ID                 string     `json:"id"`
	WorkspaceID        string     `json:"workspaceId"`
	GoalID             string     `json:"goalId,omitempty"`
	DoDID              string     `json:"doDId,omitempty"`
	Lineage            RunLineage `json:"lineage,omitempty"`
	DefinitionRevision string     `json:"definitionRevision,omitempty"`
	InitialStepKind    string     `json:"initialStepKind,omitempty"`
	State              string     `json:"state"`
	CurrentStepID      string     `json:"currentStepId,omitempty"`
	CurrentStepKind    string     `json:"currentStepKind,omitempty"`
	CurrentStepState   string     `json:"currentStepState,omitempty"`
	Priority           int        `json:"priority,omitempty"`
	BlockingTicketID   string     `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string     `json:"blockingApprovalId,omitempty"`
	FailureReason      string     `json:"failureReason,omitempty"`
	// SealedLLMTarget is copied from the persisted Runtime V2 bootstrap. It is
	// read-only history for this Run; Workspace.DefaultLLMTarget is the mutable
	// preference used only for a subsequent activation.
	SealedLLMTarget *runtimecontract.ProviderModelTarget `json:"sealedLLMTarget,omitempty"`
	CreatedAt       string                               `json:"createdAt,omitempty"`
	StartedAt       string                               `json:"startedAt,omitempty"`
	EndedAt         string                               `json:"endedAt,omitempty"`
	// RuntimeState is projection input only. It must never be serialized as an
	// application DTO or become a second authority.
	RuntimeState *runtimecontract.RuntimeStateRecord `json:"-"`
}

type RunLineage struct {
	PredecessorRunID    string `json:"predecessorRunId,omitempty"`
	PredecessorResultID string `json:"predecessorResultId,omitempty"`
	SuccessorRunID      string `json:"successorRunId,omitempty"`
	LineageState        string `json:"lineageState,omitempty"`
}

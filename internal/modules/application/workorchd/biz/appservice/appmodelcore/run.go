package appmodelcore

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Run struct {
	ID                 string                        `json:"id"`
	WorkspaceID        string                        `json:"workspaceId"`
	GoalID             string                        `json:"goalId,omitempty"`
	DoDID              string                        `json:"doDId,omitempty"`
	Lineage            projectioncontract.RunLineage `json:"lineage,omitempty"`
	DefinitionRevision string                        `json:"definitionRevision,omitempty"`
	InitialStepKind    string                        `json:"initialStepKind,omitempty"`
	State              string                        `json:"state"`
	CurrentStepID      string                        `json:"currentStepId,omitempty"`
	CurrentStepKind    string                        `json:"currentStepKind,omitempty"`
	CurrentStepState   string                        `json:"currentStepState,omitempty"`
	BlockingTicketID   string                        `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string                        `json:"blockingApprovalId,omitempty"`
	FailureReason      string                        `json:"failureReason,omitempty"`
	// SealedLLMSelection is copied from the persisted Runtime bootstrap. It is
	// read-only history for this Run; Workspace.DefaultLLMSelection is the mutable
	// preference used only for a subsequent activation.
	SealedLLMSelection *runtimecontract.ProviderInferenceSelection `json:"sealedLLMSelection,omitempty"`
	RuntimeActivation  *projectioncontract.RuntimeActivationStatus `json:"runtimeActivation,omitempty"`
	CreatedAt          string                                      `json:"createdAt,omitempty"`
	StartedAt          string                                      `json:"startedAt,omitempty"`
	EndedAt            string                                      `json:"endedAt,omitempty"`
	// RuntimeState is projection input only. It must never be serialized as an
	// application DTO or become a second authority.
	RuntimeState *runtimecontract.RuntimeStateRecord `json:"-"`
}

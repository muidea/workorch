package workorchprojectioncontract

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// Workspace is the public, owner-neutral workspace projection. Owner-private
// storage paths are deliberately excluded.
type Workspace struct {
	ID                   string                               `json:"id"`
	Title                string                               `json:"title"`
	Description          string                               `json:"description,omitempty"`
	DeliverableDirectory string                               `json:"deliverableDirectory,omitempty"`
	State                string                               `json:"state"`
	ActiveRunID          string                               `json:"activeRunId,omitempty"`
	BlockingTicketID     string                               `json:"blockingTicketId,omitempty"`
	BlockingApprovalID   string                               `json:"blockingApprovalId,omitempty"`
	TemplateID           string                               `json:"templateId,omitempty"`
	ProfileID            string                               `json:"profileId,omitempty"`
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget `json:"defaultLLMTarget,omitempty"`
	DefaultSoulID        string                               `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string                    `json:"soulBindings,omitempty"`
	ContextHints         map[string]any                       `json:"contextHints,omitempty"`
	EvidencePolicies     []map[string]any                     `json:"evidencePolicies,omitempty"`
	Projection           ProjectionStatus                     `json:"projection,omitempty"`
	ViewModel            WorkspaceViewModel                   `json:"viewModel,omitempty"`
}

type ProjectionStatus struct {
	State                   string   `json:"state"`
	StreamGeneration        string   `json:"streamGeneration,omitempty"`
	GlobalRevision          int64    `json:"globalRevision"`
	WorkspaceRevision       int64    `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64    `json:"sourceEventRevision,omitempty"`
	ProjectedSourceRevision int64    `json:"projectedSourceRevision,omitempty"`
	WorkspaceTotal          int      `json:"workspaceTotal,omitempty"`
	UpdatedAt               string   `json:"updatedAt,omitempty"`
	RebuiltAt               string   `json:"rebuiltAt,omitempty"`
	Degraded                bool     `json:"degraded,omitempty"`
	StaleReason             string   `json:"staleReason,omitempty"`
	Error                   string   `json:"error,omitempty"`
	FailedWorkspaceIds      []string `json:"failedWorkspaceIds,omitempty"`
	DegradedSince           string   `json:"degradedSince,omitempty"`
}

// Run is the public, owner-neutral run projection. RuntimeState remains an
// application-private reducer input and is intentionally absent here.
type Run struct {
	ID                 string                               `json:"id"`
	WorkspaceID        string                               `json:"workspaceId"`
	GoalID             string                               `json:"goalId,omitempty"`
	DoDID              string                               `json:"doDId,omitempty"`
	Lineage            RunLineage                           `json:"lineage,omitempty"`
	DefinitionRevision string                               `json:"definitionRevision,omitempty"`
	InitialStepKind    string                               `json:"initialStepKind,omitempty"`
	State              string                               `json:"state"`
	CurrentStepID      string                               `json:"currentStepId,omitempty"`
	CurrentStepKind    string                               `json:"currentStepKind,omitempty"`
	CurrentStepState   string                               `json:"currentStepState,omitempty"`
	Priority           int                                  `json:"priority,omitempty"`
	BlockingTicketID   string                               `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string                               `json:"blockingApprovalId,omitempty"`
	FailureReason      string                               `json:"failureReason,omitempty"`
	SealedLLMTarget    *runtimecontract.ProviderModelTarget `json:"sealedLLMTarget,omitempty"`
	RuntimeActivation  *RuntimeActivationStatus             `json:"runtimeActivation,omitempty"`
	CreatedAt          string                               `json:"createdAt,omitempty"`
	StartedAt          string                               `json:"startedAt,omitempty"`
	EndedAt            string                               `json:"endedAt,omitempty"`
}

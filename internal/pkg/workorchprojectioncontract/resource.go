package workorchprojectioncontract

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// Workspace is the public, owner-neutral workspace projection. Owner-private
// storage paths are deliberately excluded.
type Workspace struct {
	Revision             uint64                                      `json:"revision"`
	ID                   string                                      `json:"id"`
	Title                string                                      `json:"title"`
	Description          string                                      `json:"description,omitempty"`
	DeliverableDirectory string                                      `json:"deliverableDirectory,omitempty"`
	State                string                                      `json:"state"`
	ActiveRunID          string                                      `json:"activeRunId,omitempty"`
	BlockingTicketID     string                                      `json:"blockingTicketId,omitempty"`
	BlockingApprovalID   string                                      `json:"blockingApprovalId,omitempty"`
	TemplateID           string                                      `json:"templateId,omitempty"`
	ProfileID            string                                      `json:"profileId,omitempty"`
	DefaultLLMSelection  *runtimecontract.ProviderInferenceSelection `json:"defaultLLMSelection,omitempty"`
	DefaultSoulID        string                                      `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string                           `json:"soulBindings,omitempty"`
	ContextHints         map[string]any                              `json:"contextHints,omitempty"`
	EvidencePolicies     []map[string]any                            `json:"evidencePolicies,omitempty"`
	Projection           ProjectionStatus                            `json:"projection,omitempty"`
	ViewModel            WorkspaceViewModel                          `json:"viewModel,omitempty"`
}

func CloneWorkspace(item Workspace) Workspace {
	item.DefaultLLMSelection = runtimecontract.CloneProviderInferenceSelection(item.DefaultLLMSelection)
	if item.SoulBindings != nil {
		bindings := make(map[string]string, len(item.SoulBindings))
		for stage, soulID := range item.SoulBindings {
			bindings[stage] = soulID
		}
		item.SoulBindings = bindings
	}
	item.ContextHints = cloneJSONMap(item.ContextHints)
	item.EvidencePolicies = cloneJSONMaps(item.EvidencePolicies)
	item.Projection.FailedWorkspaceIds = append([]string(nil), item.Projection.FailedWorkspaceIds...)
	item.ViewModel = CloneWorkspaceViewModel(item.ViewModel)
	return item
}

func CloneWorkspaces(items []Workspace) []Workspace {
	if items == nil {
		return nil
	}
	ret := make([]Workspace, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneWorkspace(item))
	}
	return ret
}

type ProjectionStatus struct {
	State                   string   `json:"state"`
	StreamGeneration        string   `json:"streamGeneration,omitempty"`
	GlobalRevision          int64    `json:"globalRevision"`
	WorkspaceRevision       int64    `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64    `json:"sourceEventRevision"`
	ProjectedSourceRevision int64    `json:"projectedSourceRevision"`
	WorkspaceTotal          int      `json:"workspaceTotal"`
	UpdatedAt               string   `json:"updatedAt,omitempty"`
	RebuiltAt               string   `json:"rebuiltAt,omitempty"`
	Degraded                bool     `json:"degraded,omitempty"`
	StaleReason             string   `json:"staleReason,omitempty"`
	Error                   string   `json:"error,omitempty"`
	FailedWorkspaceIds      []string `json:"failedWorkspaceIds,omitempty"`
	DegradedSince           string   `json:"degradedSince,omitempty"`
}

func CloneProjectionStatus(item ProjectionStatus) ProjectionStatus {
	item.FailedWorkspaceIds = append([]string(nil), item.FailedWorkspaceIds...)
	return item
}

func ProjectionStatusFromWorkspaceViewModel(item WorkspaceViewModel) ProjectionStatus {
	return ProjectionStatus{
		State: item.State, StreamGeneration: item.StreamGeneration, GlobalRevision: item.GlobalRevision,
		WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision,
		ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt,
		Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error,
		FailedWorkspaceIds: append([]string(nil), item.FailedWorkspaceIds...), DegradedSince: item.DegradedSince,
	}
}

// Run is the public, owner-neutral run projection. RuntimeState remains an
// application-private reducer input and is intentionally absent here.
type Run struct {
	ID                 string                                      `json:"id"`
	WorkspaceID        string                                      `json:"workspaceId"`
	GoalID             string                                      `json:"goalId,omitempty"`
	DoDID              string                                      `json:"doDId,omitempty"`
	Lineage            RunLineage                                  `json:"lineage,omitempty"`
	DefinitionRevision string                                      `json:"definitionRevision,omitempty"`
	InitialStepKind    string                                      `json:"initialStepKind,omitempty"`
	State              string                                      `json:"state"`
	CurrentStepID      string                                      `json:"currentStepId,omitempty"`
	CurrentStepKind    string                                      `json:"currentStepKind,omitempty"`
	CurrentStepState   string                                      `json:"currentStepState,omitempty"`
	Priority           int                                         `json:"priority,omitempty"`
	BlockingTicketID   string                                      `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string                                      `json:"blockingApprovalId,omitempty"`
	FailureReason      string                                      `json:"failureReason,omitempty"`
	SealedLLMSelection *runtimecontract.ProviderInferenceSelection `json:"sealedLLMSelection,omitempty"`
	RuntimeActivation  *RuntimeActivationStatus                    `json:"runtimeActivation,omitempty"`
	CreatedAt          string                                      `json:"createdAt,omitempty"`
	StartedAt          string                                      `json:"startedAt,omitempty"`
	EndedAt            string                                      `json:"endedAt,omitempty"`
}

func CloneRun(item Run) Run {
	item.SealedLLMSelection = runtimecontract.CloneProviderInferenceSelection(item.SealedLLMSelection)
	item.RuntimeActivation = CloneRuntimeActivationStatus(item.RuntimeActivation)
	return item
}

func CloneRuns(items []Run) []Run {
	if items == nil {
		return nil
	}
	ret := make([]Run, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneRun(item))
	}
	return ret
}

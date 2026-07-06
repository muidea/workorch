package appmodelcore

type Run struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	GoalID             string         `json:"goalId,omitempty"`
	DoDID              string         `json:"doDId,omitempty"`
	Lineage            RunLineage     `json:"lineage,omitempty"`
	DefinitionRevision string         `json:"definitionRevision,omitempty"`
	InitialStepKind    string         `json:"initialStepKind,omitempty"`
	ExecutionContext   map[string]any `json:"executionContext,omitempty"`
	State              string         `json:"state"`
	CurrentStepID      string         `json:"currentStepId,omitempty"`
	CurrentStepKind    string         `json:"currentStepKind,omitempty"`
	CurrentStepState   string         `json:"currentStepState,omitempty"`
	Priority           int            `json:"priority,omitempty"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	FailureReason      string         `json:"failureReason,omitempty"`
	CreatedAt          string         `json:"createdAt,omitempty"`
	StartedAt          string         `json:"startedAt,omitempty"`
	EndedAt            string         `json:"endedAt,omitempty"`
}

type RunLineage struct {
	PredecessorRunID string `json:"predecessorRunId,omitempty"`
	SuccessorRunID   string `json:"successorRunId,omitempty"`
	LineageState     string `json:"lineageState,omitempty"`
}

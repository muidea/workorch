package workorchprojectioncontract

type WorkspaceViewModel struct {
	WorkspaceID             string                               `json:"workspaceId,omitempty"`
	State                   string                               `json:"state,omitempty"`
	PromptMode              string                               `json:"promptMode,omitempty"`
	Revision                int64                                `json:"revision,omitempty"`
	StreamGeneration        string                               `json:"streamGeneration,omitempty"`
	GlobalRevision          int64                                `json:"globalRevision,omitempty"`
	WorkspaceRevision       int64                                `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64                                `json:"sourceEventRevision,omitempty"`
	ProjectedSourceRevision int64                                `json:"projectedSourceRevision,omitempty"`
	UpdatedAt               string                               `json:"updatedAt,omitempty"`
	RebuiltAt               string                               `json:"rebuiltAt,omitempty"`
	Degraded                bool                                 `json:"degraded,omitempty"`
	StaleReason             string                               `json:"staleReason,omitempty"`
	Error                   string                               `json:"error,omitempty"`
	FailedWorkspaceIds      []string                             `json:"failedWorkspaceIds,omitempty"`
	DegradedSince           string                               `json:"degradedSince,omitempty"`
	Sections                map[string]WorkspaceViewModelSection `json:"sections,omitempty"`
}

type WorkspaceViewModelSection struct {
	Name                    string `json:"name"`
	Revision                int64  `json:"revision,omitempty"`
	WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64  `json:"sourceEventRevision,omitempty"`
	ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
	State                   string `json:"state,omitempty"`
}

func CloneWorkspaceViewModel(item WorkspaceViewModel) WorkspaceViewModel {
	item.FailedWorkspaceIds = append([]string(nil), item.FailedWorkspaceIds...)
	if item.Sections != nil {
		sections := make(map[string]WorkspaceViewModelSection, len(item.Sections))
		for name, section := range item.Sections {
			sections[name] = section
		}
		item.Sections = sections
	}
	return item
}

type WorkspaceActivityKind string

const (
	WorkspaceActivityRuntimeTransition WorkspaceActivityKind = "runtime_transition"
	WorkspaceActivityDispatch          WorkspaceActivityKind = "dispatch"
	WorkspaceActivityEvidence          WorkspaceActivityKind = "evidence"
	WorkspaceActivityGovernance        WorkspaceActivityKind = "governance"
	WorkspaceActivityArtifact          WorkspaceActivityKind = "artifact"
	WorkspaceActivityPatch             WorkspaceActivityKind = "patch"
	WorkspaceActivityMemory            WorkspaceActivityKind = "memory"
	WorkspaceActivityResult            WorkspaceActivityKind = "result"
	WorkspaceActivityFailure           WorkspaceActivityKind = "failure"
)

type WorkspaceActivityItem struct {
	ID                   string                `json:"id"`
	WorkspaceID          string                `json:"workspaceId"`
	RunID                string                `json:"runId,omitempty"`
	StepID               string                `json:"stepId,omitempty"`
	Kind                 WorkspaceActivityKind `json:"kind"`
	State                string                `json:"state,omitempty"`
	Title                string                `json:"title,omitempty"`
	Summary              string                `json:"summary,omitempty"`
	OccurredAt           string                `json:"occurredAt,omitempty"`
	SourceEventRevision  int64                 `json:"sourceEventRevision,omitempty"`
	RuntimeStateRevision uint64                `json:"runtimeStateRevision,omitempty"`
	EvidenceSequence     uint64                `json:"evidenceSequence,omitempty"`
}

type WorkspaceActivityFeed struct {
	StreamGeneration string                  `json:"streamGeneration,omitempty"`
	WorkspaceID      string                  `json:"workspaceId"`
	DisplayRunID     string                  `json:"displayRunId,omitempty"`
	ActivityRevision int64                   `json:"activityRevision,omitempty"`
	Items            []WorkspaceActivityItem `json:"items,omitempty"`
	Truncated        bool                    `json:"truncated,omitempty"`
	NextCursor       string                  `json:"nextCursor,omitempty"`
}

func CloneWorkspaceActivityFeed(item WorkspaceActivityFeed) WorkspaceActivityFeed {
	item.Items = append([]WorkspaceActivityItem(nil), item.Items...)
	return item
}

type RunCommandStatus struct {
	WorkspaceID        string `json:"workspaceId,omitempty"`
	RunID              string `json:"runId,omitempty"`
	State              string `json:"state,omitempty"`
	Intent             string `json:"intent,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Disposition        string `json:"disposition,omitempty"`
	Diagnostic         string `json:"diagnostic,omitempty"`
	RequestID          string `json:"requestId,omitempty"`
	IdempotencyKey     string `json:"idempotencyKey,omitempty"`
	Source             string `json:"source,omitempty"`
	DefinitionRevision string `json:"definitionRevision,omitempty"`
	DecisionID         string `json:"decisionId,omitempty"`
	QueuedAt           string `json:"queuedAt,omitempty"`
	AcceptedAt         string `json:"acceptedAt,omitempty"`
	RejectedAt         string `json:"rejectedAt,omitempty"`
	Error              string `json:"error,omitempty"`
	Revision           int64  `json:"revision,omitempty"`
}

type WorkspaceRuntimeView struct {
	WorkspaceID       string                   `json:"workspaceId"`
	ActiveRunID       string                   `json:"activeRunId,omitempty"`
	DisplayRunID      string                   `json:"displayRunId,omitempty"`
	Projection        WorkspaceViewModel       `json:"projection"`
	Revision          WorkspaceRefreshRevision `json:"revision"`
	PromptMode        string                   `json:"promptMode,omitempty"`
	RunState          string                   `json:"runState,omitempty"`
	TaskProgress      TaskProgress             `json:"taskProgress"`
	RunHeartbeat      RunHeartbeat             `json:"runHeartbeat"`
	ConvergenceStatus *ConvergenceStatus       `json:"convergenceStatus,omitempty"`
	Acceptance        AcceptanceProjection     `json:"acceptance"`
	RunCommand        *RunCommandStatus        `json:"runCommand,omitempty"`
	BlockingDecision  *Decision                `json:"blockingDecision,omitempty"`
	BlockingApproval  *Approval                `json:"blockingApproval,omitempty"`
	Outcome           WorkspaceOutcome         `json:"outcome"`
	Activity          WorkspaceActivityFeed    `json:"activity"`
}

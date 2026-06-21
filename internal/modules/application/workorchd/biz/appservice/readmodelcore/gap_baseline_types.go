package readmodelcore

type GapBaseline struct {
	WorkspaceID           string           `json:"workspaceId,omitempty"`
	RunID                 string           `json:"runId,omitempty"`
	DefinitionRevision    string           `json:"definitionRevision,omitempty"`
	BaselineRevision      string           `json:"baselineRevision,omitempty"`
	CurrentFocus          map[string]any   `json:"currentFocus,omitempty"`
	OpenCoverageWorkItems []map[string]any `json:"openCoverageWorkItems,omitempty"`
	AcceptedEvidenceRefs  []string         `json:"acceptedEvidenceRefs,omitempty"`
	RejectedEvidenceRefs  []string         `json:"rejectedEvidenceRefs,omitempty"`
	TodoItems             []GapTodoItem    `json:"todoItems,omitempty"`
	NextAction            GapNextAction    `json:"nextAction,omitempty"`
}

type GapTodoItem struct {
	ID                    string         `json:"id,omitempty"`
	Source                string         `json:"source,omitempty"`
	SourceRef             string         `json:"sourceRef,omitempty"`
	CoverageWorkItemIDs   []string       `json:"coverageWorkItemIds,omitempty"`
	RequiredEvidenceKinds []string       `json:"requiredEvidenceKinds,omitempty"`
	Status                string         `json:"status,omitempty"`
	Priority              int            `json:"priority,omitempty"`
	Reason                string         `json:"reason,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
}

type GapNextAction struct {
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason,omitempty"`
}

package workorchprojectioncontract

// GapBaseline is a read-only projection of the gap already derived from the
// Runtime aggregate. CurrentFocus, coverage items and metadata preserve the
// original diagnostic payload, but cannot be used to rebuild authority or
// select a Runtime transition.
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
	Diagnostics           []GapDiagnostic  `json:"diagnostics,omitempty"`
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

// GapNextAction reports the action label already selected by the Runtime
// owner. It is display data, not a command or reducer input.
type GapNextAction struct {
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type GapDiagnostic struct {
	Code             string `json:"code,omitempty"`
	Severity         string `json:"severity,omitempty"`
	Kind             string `json:"kind,omitempty"`
	Source           string `json:"source,omitempty"`
	Candidate        string `json:"candidate,omitempty"`
	CurrentAuthority string `json:"currentAuthority,omitempty"`
	Message          string `json:"message,omitempty"`
}

func CloneGapBaseline(item *GapBaseline) *GapBaseline {
	if item == nil {
		return nil
	}
	ret := *item
	ret.CurrentFocus = cloneJSONMap(item.CurrentFocus)
	ret.OpenCoverageWorkItems = cloneJSONMaps(item.OpenCoverageWorkItems)
	ret.AcceptedEvidenceRefs = append([]string(nil), item.AcceptedEvidenceRefs...)
	ret.RejectedEvidenceRefs = append([]string(nil), item.RejectedEvidenceRefs...)
	if item.TodoItems != nil {
		ret.TodoItems = make([]GapTodoItem, 0, len(item.TodoItems))
		for _, todo := range item.TodoItems {
			todo.CoverageWorkItemIDs = append([]string(nil), todo.CoverageWorkItemIDs...)
			todo.RequiredEvidenceKinds = append([]string(nil), todo.RequiredEvidenceKinds...)
			todo.Metadata = cloneJSONMap(todo.Metadata)
			ret.TodoItems = append(ret.TodoItems, todo)
		}
	}
	ret.Diagnostics = CloneGapDiagnostics(item.Diagnostics)
	return &ret
}

func CloneGapDiagnostics(items []GapDiagnostic) []GapDiagnostic {
	return append([]GapDiagnostic(nil), items...)
}

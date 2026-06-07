package readmodelcore

type TaskDecomposition struct {
	ID            string                  `json:"id"`
	WorkspaceID   string                  `json:"workspaceId"`
	RunID         string                  `json:"runId"`
	RootStepID    string                  `json:"rootStepId"`
	State         string                  `json:"state"`
	Summary       string                  `json:"summary,omitempty"`
	GraphBranch   string                  `json:"graphBranch,omitempty"`
	NodeTotal     int                     `json:"nodeTotal"`
	NodeCompleted int                     `json:"nodeCompleted"`
	NodeFailed    int                     `json:"nodeFailed"`
	NodeRunning   int                     `json:"nodeRunning"`
	Nodes         []TaskDecompositionNode `json:"nodes,omitempty"`
	EvidenceRefs  []string                `json:"evidenceRefs,omitempty"`
}

type TaskDecompositionNode struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Title        string         `json:"title,omitempty"`
	State        string         `json:"state"`
	DependsOn    []string       `json:"dependsOn,omitempty"`
	Branch       string         `json:"branch,omitempty"`
	StepID       string         `json:"stepId,omitempty"`
	StepState    string         `json:"stepState,omitempty"`
	EvidenceRefs []string       `json:"evidenceRefs,omitempty"`
	Input        map[string]any `json:"input,omitempty"`
}

type TaskPlan struct {
	ID               string   `json:"id"`
	WorkspaceID      string   `json:"workspaceId"`
	RunID            string   `json:"runId"`
	AnchorStepID     string   `json:"anchorStepId"`
	RevisionStepID   string   `json:"revisionStepId"`
	Revision         int      `json:"revision"`
	State            string   `json:"state"`
	Stage            string   `json:"stage,omitempty"`
	Summary          string   `json:"summary,omitempty"`
	GraphBranch      string   `json:"graphBranch,omitempty"`
	NodeTotal        int      `json:"nodeTotal"`
	NodeCompleted    int      `json:"nodeCompleted"`
	NodeFailed       int      `json:"nodeFailed"`
	NodeRunning      int      `json:"nodeRunning"`
	EvidenceRefs     []string `json:"evidenceRefs,omitempty"`
	OccurredAt       string   `json:"occurredAt,omitempty"`
	SourceProjection string   `json:"sourceProjection,omitempty"`
}

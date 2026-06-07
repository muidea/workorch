package readmodelcore

type RunOutcome struct {
	RunID        string   `json:"runId,omitempty"`
	State        string   `json:"state"`
	RawState     string   `json:"rawState,omitempty"`
	Accepted     bool     `json:"accepted"`
	Terminal     bool     `json:"terminal"`
	ResultID     string   `json:"resultId,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
}

type WorkspaceOutcome struct {
	WorkspaceID       string   `json:"workspaceId,omitempty"`
	State             string   `json:"state"`
	RawWorkspaceState string   `json:"rawWorkspaceState,omitempty"`
	RunID             string   `json:"runId,omitempty"`
	ResultID          string   `json:"resultId,omitempty"`
	Accepted          bool     `json:"accepted"`
	Terminal          bool     `json:"terminal"`
	Summary           string   `json:"summary,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	EvidenceRefs      []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs      []string `json:"artifactRefs,omitempty"`
}

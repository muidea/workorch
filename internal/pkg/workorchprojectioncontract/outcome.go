package workorchprojectioncontract

// RunDisplayOutcome is a bounded display summary derived from canonical Run and
// formal Result projections. It is not a Result owner record and must never be
// translated into Start/Resume/Restart or lifecycle mutation intent.
type RunDisplayOutcome struct {
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

// WorkspaceDisplayOutcome is the workspace-level display counterpart of RunDisplayOutcome.
// Formal accepted output remains the separately projected Result resource.
type WorkspaceDisplayOutcome struct {
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

func CloneWorkspaceDisplayOutcome(item WorkspaceDisplayOutcome) WorkspaceDisplayOutcome {
	item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	item.ArtifactRefs = append([]string(nil), item.ArtifactRefs...)
	return item
}

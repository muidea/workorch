package readmodelcore

type Artifact struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId,omitempty"`
	ResultID           string         `json:"resultId,omitempty"`
	StepID             string         `json:"stepId,omitempty"`
	ToolCallID         string         `json:"toolCallId,omitempty"`
	PatchID            string         `json:"patchId,omitempty"`
	ValidationResultID string         `json:"validationResultId,omitempty"`
	ArtifactType       string         `json:"artifactType"`
	Category           string         `json:"category,omitempty"`
	Name               string         `json:"name,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	URI                string         `json:"uri,omitempty"`
	ContentType        string         `json:"contentType,omitempty"`
	State              string         `json:"state"`
	EvidenceRefs       []string       `json:"evidenceRefs,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          string         `json:"createdAt,omitempty"`
	UpdatedAt          string         `json:"updatedAt,omitempty"`
}

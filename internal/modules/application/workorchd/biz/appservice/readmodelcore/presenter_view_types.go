package readmodelcore

type StepView struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	State  string         `json:"state"`
	Input  map[string]any `json:"input,omitempty"`
	Output map[string]any `json:"output,omitempty"`
}

type ObservedView struct {
	ID           string         `json:"id"`
	Version      string         `json:"version"`
	Topic        string         `json:"topic"`
	OccurredAt   string         `json:"occurredAt"`
	SourceModule string         `json:"sourceModule"`
	SubjectType  string         `json:"subjectType"`
	SubjectID    string         `json:"subjectId"`
	WorkspaceID  string         `json:"workspaceId"`
	RunID        string         `json:"runId"`
	StepID       string         `json:"stepId"`
	Payload      map[string]any `json:"payload"`
}

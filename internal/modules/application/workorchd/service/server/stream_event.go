package server

// streamEvent is the HTTP/SSE presentation DTO. It deliberately does not
// implement or alias the component EventHub Envelope contract.
type streamEvent struct {
	ID           string         `json:"id"`
	Version      string         `json:"version"`
	Topic        string         `json:"topic"`
	OccurredAt   string         `json:"occurredAt,omitempty"`
	SourceModule string         `json:"sourceModule,omitempty"`
	SubjectType  string         `json:"subjectType,omitempty"`
	SubjectID    string         `json:"subjectId,omitempty"`
	WorkspaceID  string         `json:"workspaceId,omitempty"`
	RunID        string         `json:"runId,omitempty"`
	StepID       string         `json:"stepId,omitempty"`
	Revision     int64          `json:"revision,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

package readmodelcore

type RuntimeCheckpoint struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId"`
	CheckpointType     string         `json:"checkpointType"`
	CurrentStepID      string         `json:"currentStepId,omitempty"`
	RunStateSnapshot   string         `json:"runStateSnapshot"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	ThreadRef          string         `json:"threadRef,omitempty"`
	OccurredAt         string         `json:"occurredAt,omitempty"`
	RestorableUntil    string         `json:"restorableUntil,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

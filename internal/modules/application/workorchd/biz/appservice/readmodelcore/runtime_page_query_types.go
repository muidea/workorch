package readmodelcore

type RuntimeCheckpointQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type RuntimeCheckpointPage struct {
	RunID              string              `json:"runId"`
	RuntimeCheckpoints []RuntimeCheckpoint `json:"runtimeCheckpoints,omitempty"`
	Limit              int                 `json:"limit"`
	CursorAfterID      string              `json:"cursorAfterId,omitempty"`
	NextCursorAfterID  string              `json:"nextCursorAfterId,omitempty"`
	HasMore            bool                `json:"hasMore"`
}

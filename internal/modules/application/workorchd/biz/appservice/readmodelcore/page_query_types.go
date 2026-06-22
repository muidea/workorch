package readmodelcore

type StepQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type StepPage struct {
	RunID             string     `json:"runId"`
	Steps             []StepView `json:"steps,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}

type ToolCallQuery struct {
	WorkspaceID   string
	RunID         string
	CursorAfterID string
	Limit         int
}

type ToolCallPage struct {
	RunID             string     `json:"runId"`
	ToolCalls         []ToolCall `json:"toolCalls,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}

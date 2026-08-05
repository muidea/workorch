package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

type StepPage struct {
	RunID             string                        `json:"runId"`
	Steps             []projectioncontract.StepView `json:"steps,omitempty"`
	Limit             int                           `json:"limit"`
	CursorAfterID     string                        `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string                        `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                          `json:"hasMore"`
}

type ToolCallPage struct {
	RunID             string                        `json:"runId"`
	ToolCalls         []projectioncontract.ToolCall `json:"toolCalls,omitempty"`
	Limit             int                           `json:"limit"`
	CursorAfterID     string                        `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string                        `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                          `json:"hasMore"`
}

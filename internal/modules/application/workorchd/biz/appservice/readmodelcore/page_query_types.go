package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

type StepPage struct {
	RunID string                        `json:"runId"`
	Steps []projectioncontract.StepView `json:"steps,omitempty"`
	projectioncontract.CursorPage
}

type ToolCallPage struct {
	RunID     string                        `json:"runId"`
	ToolCalls []projectioncontract.ToolCall `json:"toolCalls,omitempty"`
	projectioncontract.CursorPage
}

package run

import "github.com/muidea/workorch/internal/pkg/common"

type ToolCall struct {
	ID         common.ToolCallID
	WorkspaceID common.WorkspaceID
	RunID      common.RunID
	StepID     common.StepID
	ToolName   string
	Arguments  map[string]any
	State      common.ToolCallState
	StartedAt  string
	EndedAt    string
	ExitSummary string
	Output     map[string]any
	Error      string
}

package run

import "github.com/muidea/workorch/internal/pkg/common"
import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

type ToolCall struct {
	ID          common.ToolCallID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	ToolName    string
	Arguments   workorcheventpayload.Payload
	State       common.ToolCallState
	StartedAt   string
	EndedAt     string
	ExitSummary string
	Output      workorcheventpayload.Payload
	Error       string
}

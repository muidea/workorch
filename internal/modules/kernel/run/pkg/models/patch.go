package run

import "github.com/muidea/workorch/internal/pkg/common"

type Patch struct {
	ID         common.PatchID
	WorkspaceID common.WorkspaceID
	RunID      common.RunID
	StepID     common.StepID
	ToolCallID common.ToolCallID
	PatchType  string
	TargetRef  string
	Summary    string
	State      common.PatchState
	CreatedAt  string
	AppliedAt  string
}

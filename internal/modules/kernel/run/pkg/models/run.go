package run

import "github.com/muidea/workorch/internal/pkg/common"

type Run struct {
	ID          common.RunID
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
	DoDID       common.DoDID
	State       common.RunState
}

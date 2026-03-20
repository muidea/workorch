package contracts

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateRunCommand struct {
	RunID       common.RunID
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
	DoDID       common.DoDID
}

type StartRunCommand struct{ RunID common.RunID }
type CompleteRunCommand struct{ RunID common.RunID }
type FailRunCommand struct {
	RunID  common.RunID
	Reason string
}

type RunService interface {
	CreateRun(context.Context, CreateRunCommand) error
	StartRun(context.Context, StartRunCommand) error
	CompleteRun(context.Context, CompleteRunCommand) error
	FailRun(context.Context, FailRunCommand) error
}

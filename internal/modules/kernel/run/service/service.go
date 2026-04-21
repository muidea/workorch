package run

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
type ResumeRunCommand struct{ RunID common.RunID }

type BlockOnDecisionCommand struct {
	RunID    common.RunID
	TicketID common.TicketID
}

type BlockOnApprovalCommand struct {
	RunID      common.RunID
	ApprovalID common.ApprovalID
}

type CompleteRunCommand struct{ RunID common.RunID }

type FailRunCommand struct {
	RunID  common.RunID
	Reason string
}

type Service interface {
	CreateRun(context.Context, CreateRunCommand) error
	StartRun(context.Context, StartRunCommand) error
	ResumeRun(context.Context, ResumeRunCommand) error
	BlockOnDecision(context.Context, BlockOnDecisionCommand) error
	BlockOnApproval(context.Context, BlockOnApprovalCommand) error
	CompleteRun(context.Context, CompleteRunCommand) error
	FailRun(context.Context, FailRunCommand) error
}

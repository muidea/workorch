package run

import "github.com/muidea/workorch/internal/pkg/common"

type Run struct {
	ID                 common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	InitialStepKind    string
	ExecutionContext   map[string]any
	State              common.RunState
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
}

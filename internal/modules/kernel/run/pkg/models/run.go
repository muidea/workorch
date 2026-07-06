package run

import "github.com/muidea/workorch/internal/pkg/common"

type Run struct {
	ID                 common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	Lineage            RunLineage
	DefinitionRevision string
	InitialStepKind    string
	ExecutionContext   map[string]any
	State              common.RunState
	Priority           int
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
	CreatedAt          string
	StartedAt          string
	EndedAt            string
}

type RunLineage struct {
	PredecessorRunID common.RunID
	SuccessorRunID   common.RunID
	LineageState     string
}

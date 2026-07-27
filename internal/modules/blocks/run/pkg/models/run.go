package run

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Run struct {
	ID                     common.RunID
	WorkspaceID            common.WorkspaceID
	GoalID                 common.GoalID
	DoDID                  common.DoDID
	Lineage                RunLineage
	DefinitionRevision     string
	InitialStepKind        string
	RuntimeBootstrap       *runtimecontract.RuntimeBootstrap
	RuntimeContractVersion string
	RuntimeState           *runtimecontract.RuntimeStateRecord
	State                  common.RunState
	Priority               int
	BlockingTicketID       common.TicketID
	BlockingApprovalID     common.ApprovalID
	FailureReason          string
	CreatedAt              string
	StartedAt              string
	EndedAt                string
}

type RunLineage struct {
	PredecessorRunID    common.RunID
	PredecessorResultID common.ResultID
	SuccessorRunID      common.RunID
	LineageState        string
}

package run

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Run struct {
	// Revision is the owner-local persistence revision. Lifecycle and Runtime
	// aggregate writes compare it before replacing the record.
	Revision               uint64
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
	// TerminalDelivery is the Run owner's immutable SM-1 terminal fence.
	TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity
}

type RunLineage struct {
	PredecessorRunID    common.RunID
	PredecessorResultID common.ResultID
	SuccessorRunID      common.RunID
	LineageState        string
}

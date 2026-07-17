package contracts

import (
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CreateRunCommand struct {
	RunID              common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	DefinitionRevision string
	InitialStepKind    string
	RuntimeBootstrap   *runtimemodel.RuntimeBootstrap
	Lineage            runmodel.RunLineage
	Priority           int
}

// LinkRunSuccessorCommand links predecessor.SuccessorRunID to a successor Run under CAS semantics.
// Empty ExpectedSuccessorRunID means only empty successor may be set; same successor is idempotent.
type LinkRunSuccessorCommand struct {
	WorkspaceID            common.WorkspaceID
	PredecessorRunID       common.RunID
	SuccessorRunID         common.RunID
	ExpectedSuccessorRunID common.RunID
}

type StartRunCommand struct{ RunID common.RunID }

type ResumeRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type UpdateRunCommand struct {
	RunID                        common.RunID
	WorkspaceID                  common.WorkspaceID
	GoalID                       common.GoalID
	DoDID                        common.DoDID
	Lineage                      runmodel.RunLineage
	DefinitionRevision           string
	InitialStepKind              string
	RuntimeBootstrap             *runtimemodel.RuntimeBootstrap
	RuntimeContractVersion       string
	RuntimeState                 *runtimemodel.RuntimeStateRecord
	ExpectedRuntimeStateRevision *uint64
	State                        common.RunState
	Priority                     int
	BlockingTicketID             common.TicketID
	BlockingApprovalID           common.ApprovalID
	FailureReason                string
	CreatedAt                    string
	StartedAt                    string
	EndedAt                      string
}

type BlockOnDecisionCommand struct {
	RunID    common.RunID
	TicketID common.TicketID
}

type BlockOnApprovalCommand struct {
	RunID      common.RunID
	ApprovalID common.ApprovalID
}

type CancelRunCommand struct{ RunID common.RunID }
type SupersedeRunCommand struct{ RunID common.RunID }
type DeleteRunCommand struct{ RunID common.RunID }

type FailRunCommand struct {
	RunID  common.RunID
	Reason string
}

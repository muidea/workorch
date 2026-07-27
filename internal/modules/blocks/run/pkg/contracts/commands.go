package contracts

import (
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
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
	// UpdateRunCommand is the sole generic persistence command for the
	// Runtime V2 aggregate. It intentionally excludes SM-1 lifecycle and
	// immutable definition fields; those are owned by dedicated Run reducers.
	RunID                        common.RunID
	WorkspaceID                  common.WorkspaceID
	RuntimeState                 *runtimemodel.RuntimeStateRecord
	ExpectedRuntimeStateRevision *uint64
	// ExpectedModelRevision fences the complete Run record, including SM-1
	// lifecycle fields that are deliberately absent from this generic command.
	ExpectedModelRevision *uint64
}

type BlockOnDecisionCommand struct {
	RunID    common.RunID
	TicketID common.TicketID
}

type BlockOnApprovalCommand struct {
	RunID      common.RunID
	ApprovalID common.ApprovalID
}

type DeleteRunCommand struct{ RunID common.RunID }

type FailRunCommand struct {
	RunID  common.RunID
	Reason string
}

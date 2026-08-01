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

type UpdateRunCommand struct {
	// UpdateRunCommand is the opaque persistence command for the Runtime
	// aggregate. It intentionally excludes SM-1 lifecycle and immutable
	// definition fields; those are owned by dedicated Run reducers. The only
	// semantic CAS fence is ExpectedRuntimeStateRevision. A Run model revision
	// is a physical-record revision and must not fence SM-2/SM-3/SM-4 progress.
	RunID                        common.RunID
	WorkspaceID                  common.WorkspaceID
	RuntimeState                 *runtimemodel.RuntimeStateRecord
	ExpectedRuntimeStateRevision *uint64
}

type DeleteRunCommand struct{ RunID common.RunID }

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
	ExecutionContext   map[string]any
	Priority           int
}

type StartRunCommand struct{ RunID common.RunID }

type ResumeRunCommand struct {
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	LoadedRun             *runmodel.Run
	ExecutionContextPatch map[string]any
}

type RecoverRuntimeGapCommand struct {
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	StepID                common.StepID
	StepKind              string
	Gap                   string
	RequestID             string
	QueueKey              string
	LatestStepTime        string
	RecoveryPolicy        runtimemodel.RuntimeRecoveryPolicy
	RecoverySafety        string
	Reason                string
	ExecutionContextPatch map[string]any
}

type UpdateRunCommand struct {
	RunID              common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	DefinitionRevision string
	InitialStepKind    string
	ExecutionContext   map[string]any
	State              common.RunState
	Priority           int
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
}

type BlockOnDecisionCommand struct {
	RunID    common.RunID
	TicketID common.TicketID
}

type BlockOnApprovalCommand struct {
	RunID      common.RunID
	ApprovalID common.ApprovalID
}

type CompleteRunCommand struct{ RunID common.RunID }
type ValidateRunCommand struct{ RunID common.RunID }
type CancelRunCommand struct{ RunID common.RunID }
type SupersedeRunCommand struct{ RunID common.RunID }
type DeleteRunCommand struct{ RunID common.RunID }

type FailRunCommand struct {
	RunID  common.RunID
	Reason string
}

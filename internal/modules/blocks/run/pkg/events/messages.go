package events

import (
	"fmt"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/blocks/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateRunCommand = runcontracts.CreateRunCommand
type LinkRunSuccessorCommand = runcontracts.LinkRunSuccessorCommand
type EnsureRunCommand = runcontracts.CreateRunCommand
type UpdateRunCommand = runcontracts.UpdateRunCommand

type DeleteRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type GetRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type ListRunsCommand struct {
	States  []common.RunState
	Compact bool
}

type ListRunsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
	States      []common.RunState
	Compact     bool
}

type RuntimeV2RecoveryClass string

const (
	RuntimeV2RecoveryActiveNoDispatch        RuntimeV2RecoveryClass = "active_no_dispatch"
	RuntimeV2RecoveryActiveSealedDispatch    RuntimeV2RecoveryClass = "active_sealed_dispatch"
	RuntimeV2RecoveryBlockedGovernance       RuntimeV2RecoveryClass = "blocked_governance"
	RuntimeV2RecoveryTerminalDeliveryPending RuntimeV2RecoveryClass = "terminal_delivery_pending"
)

type QueryRuntimeV2RecoveryInventoryCommand struct {
	Cursor string
	Limit  int
}

type RuntimeV2RecoveryInventoryItem struct {
	WorkspaceID              common.WorkspaceID
	RunID                    common.RunID
	StateRevision            uint64
	Class                    RuntimeV2RecoveryClass
	ActiveDispatchID         string
	BlockingTicketID         common.TicketID
	BlockingApprovalID       common.ApprovalID
	TerminalDeliveryAttempts int
}

type QueryRuntimeV2RecoveryInventoryResult struct {
	Items      []RuntimeV2RecoveryInventoryItem
	NextCursor string
}

type GetStepCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}

type ListStepsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type ListStepSummariesByRunCommand = ListStepsByRunCommand

type GetRuntimeCheckpointCommand struct {
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	CheckpointID common.CheckpointID
}

type ListRuntimeCheckpointsByRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type PurgeRuntimeResourcesCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type RunStateChangedData struct {
	RunID                   common.RunID
	WorkspaceID             common.WorkspaceID
	GoalID                  common.GoalID
	DoDID                   common.DoDID
	State                   common.RunState
	BlockingTicketID        common.TicketID
	BlockingApprovalID      common.ApprovalID
	FailureReason           string
	CausationID             string
	DefinitionRevision      string
	EvidenceRevision        string
	RuntimeStateRevision    uint64
	RuntimeConvergenceState runtimecontract.ConvergenceState
	RuntimeAction           runtimecontract.RuntimeAction
	RuntimeFocusID          string
	AuthoritySnapshotID     string
	DispatchID              string
	// RepairAttempts is a monotonic snapshot of the persisted SM-2 ledger.
	// Consumers observe deltas only; it does not grant them transition authority.
	RepairAttempts int
}

// RunContinuationRequestedData is emitted only by Run owner after it accepts
// a scoped Continue intent for an already-active Run. It deliberately differs
// from RunStateChangedData: duplicate resume is not a fresh SM-1 transition.
type RunContinuationRequestedData struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	CausationID string
}

func (d RunContinuationRequestedData) Validate() error {
	if d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.CausationID) == "" {
		return fmt.Errorf("%w: run continuation fact requires scope and causation", common.ErrInvalidArgument)
	}
	return nil
}

func RunStateChangedDataFromModel(run *runmodel.Run) RunStateChangedData {
	if run == nil {
		return RunStateChangedData{}
	}
	ret := RunStateChangedData{
		RunID:              run.ID,
		WorkspaceID:        run.WorkspaceID,
		GoalID:             run.GoalID,
		DoDID:              run.DoDID,
		State:              run.State,
		BlockingTicketID:   run.BlockingTicketID,
		BlockingApprovalID: run.BlockingApprovalID,
		FailureReason:      run.FailureReason,
		DefinitionRevision: run.DefinitionRevision,
	}
	if run.RuntimeState != nil {
		ret.RuntimeStateRevision = run.RuntimeState.StateRevision
		ret.RuntimeConvergenceState = run.RuntimeState.Decision.State
		ret.RuntimeAction = run.RuntimeState.Decision.Action
		ret.RuntimeFocusID = run.RuntimeState.Facts.Focus.ID
		ret.AuthoritySnapshotID = run.RuntimeState.Decision.AuthoritySnapshot.ID
		if run.RuntimeState.ActiveDispatch != nil {
			ret.DispatchID = run.RuntimeState.ActiveDispatch.ID
		}
		ret.RepairAttempts = run.RuntimeState.Facts.Repair.Normalized().RepairAttempts
		ret.DefinitionRevision = run.RuntimeState.Facts.DefinitionRevision
		ret.EvidenceRevision = run.RuntimeState.Facts.EvidenceRevision
	}
	ret.CausationID = "run-state/" + string(run.ID) + "/" + fmt.Sprintf("%d", ret.RuntimeStateRevision)
	return ret
}

// Validate verifies the immutable identity carried by a Run owner
// notification. A zero RuntimeStateRevision is permitted only for the
// pre-runtime create notification; every Runtime V2 transition carries the
// persisted authority/facts identity.
func (d RunStateChangedData) Validate() error {
	if d.WorkspaceID == "" || d.RunID == "" || d.CausationID == "" {
		return fmt.Errorf("%w: run state fact requires scope and causation", common.ErrInvalidArgument)
	}
	if d.RuntimeStateRevision > 0 && (strings.TrimSpace(d.DefinitionRevision) == "" ||
		strings.TrimSpace(d.EvidenceRevision) == "" || strings.TrimSpace(d.AuthoritySnapshotID) == "") {
		return fmt.Errorf("%w: runtime run state fact requires definition, evidence and authority identity", common.ErrInvalidArgument)
	}
	return nil
}

type RunCommandResult struct {
	Run          *runmodel.Run
	Runs         []*runmodel.Run
	Step         *runmodel.Step
	Steps        []*runmodel.Step
	Checkpoint   *runmodel.RuntimeCheckpoint
	Checkpoints  []*runmodel.RuntimeCheckpoint
	DeletedCount int
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandCreate).MessageContract(),
		eventcontract.CommandTopic[EnsureRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandEnsure).MessageContract(),
		eventcontract.CommandTopic[GetRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListRunsCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandList).MessageContract(),
		eventcontract.CommandTopic[ListRunsByWorkspaceCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeV2RecoveryInventoryCommand, QueryRuntimeV2RecoveryInventoryResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandRuntimeV2RecoveryInventory).MessageContract(),
		eventcontract.CommandTopic[GetStepCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGetStep).MessageContract(),
		eventcontract.CommandTopic[ListStepsByRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListStepsByRun).MessageContract(),
		eventcontract.CommandTopic[ListStepSummariesByRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListStepSummariesByRun).MessageContract(),
		eventcontract.CommandTopic[GetRuntimeCheckpointCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGetRuntimeCheckpoint).MessageContract(),
		eventcontract.CommandTopic[ListRuntimeCheckpointsByRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListCheckpointsByRun).MessageContract(),
		eventcontract.CommandTopic[PurgeRuntimeResourcesCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandPurgeRuntimeResources).MessageContract(),
		eventcontract.CommandTopic[UpdateRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandUpdate).MessageContract(),
		eventcontract.CommandTopic[LinkRunSuccessorCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandLinkSuccessor).MessageContract(),
		eventcontract.CommandTopic[DeleteRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandDelete).MessageContract(),
		eventcontract.CommandTopic[ActivateRunCommand, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandActivate).MessageContract(),
		eventcontract.CommandTopic[ContinueRunCommand, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandContinue).MessageContract(),
		eventcontract.CommandTopic[ApplyRuntimeGovernanceWaitCommand, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandApplyGovernanceWait).MessageContract(),
		eventcontract.CommandTopic[ApplyRuntimeContinuationEligibleCommand, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandApplyContinuationEligible).MessageContract(),
		eventcontract.CommandTopic[ApplyRuntimeTerminalDeliveryCommand, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandApplyTerminalDelivery).MessageContract(),
	}
	for _, topic := range []string{
		TopicRunCreated,
		TopicRunStarted,
		TopicRunResumed,
		TopicRunBlockedOnDecision,
		TopicRunBlockedOnApproval,
		TopicRunCompleted,
		TopicRunFailed,
		TopicRunCancelled,
		TopicRunSuperseded,
		TopicRunStateChanged,
	} {
		ret = append(ret, eventcontract.DataTopic[RunStateChangedData]("run", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret, eventcontract.DataTopic[RunContinuationRequestedData]("run", definitionPackage, eventcontract.LanePolicyState, TopicRunContinuationRequested).MessageContract())
	return ret
}

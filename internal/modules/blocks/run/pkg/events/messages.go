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

type QueryRuntimeV2RecoveryInventoryCommand struct {
	Cursor string
	Limit  int
}

type RuntimeV2RecoveryInventoryItem struct {
	WorkspaceID                  common.WorkspaceID
	RunID                        common.RunID
	State                        common.RunState
	HasRuntimeBootstrap          bool
	HasRuntimeState              bool
	RuntimeStateRef              string
	RuntimeStateRevision         uint64
	RuntimeStateSchemaVersion    uint32
	RuntimeActivationDisposition runmodel.RuntimeActivationDisposition
}

type QueryRuntimeV2RecoveryInventoryResult struct {
	Items      []RuntimeV2RecoveryInventoryItem
	NextCursor string
}

// RuntimeStateUpdateDisposition is the Run owner's factual outcome for an
// opaque Runtime aggregate CAS request. Only aggregate_revision_mismatch
// authorizes Orchestrator SM-4 CAS reload; the other outcomes must not be
// collapsed into common.ErrConflict.
type RuntimeStateUpdateDisposition string

const (
	RuntimeStateUpdateDispositionAccepted                  RuntimeStateUpdateDisposition = "accepted"
	RuntimeStateUpdateDispositionAggregateRevisionMismatch RuntimeStateUpdateDisposition = "aggregate_revision_mismatch"
	RuntimeStateUpdateDispositionSemanticRejection         RuntimeStateUpdateDisposition = "semantic_rejection"
	RuntimeStateUpdateDispositionOwnerTransient            RuntimeStateUpdateDisposition = "owner_transient"
)

// RuntimeStateUpdateResult is a concrete cross-owner fact. It preserves both
// aggregate revision fence values and the bounded producer diagnostic without
// carrying decoded aggregate authority.
type RuntimeStateUpdateResult struct {
	WorkspaceID             common.WorkspaceID
	RunID                   common.RunID
	Disposition             RuntimeStateUpdateDisposition
	ExpectedRuntimeRevision uint64
	ObservedRuntimeRevision uint64
	Feedback                runtimecontract.RuntimeFeedback
}

func (r RuntimeStateUpdateResult) Validate() error {
	if r.WorkspaceID == "" || r.RunID == "" {
		return fmt.Errorf("%w: runtime state update result requires scope", common.ErrInvalidArgument)
	}
	category := runtimecontract.RuntimeFeedbackCategoryAccepted
	switch r.Disposition {
	case RuntimeStateUpdateDispositionAccepted:
	case RuntimeStateUpdateDispositionAggregateRevisionMismatch:
		category = runtimecontract.RuntimeFeedbackCategoryAggregateRevisionMismatch
	case RuntimeStateUpdateDispositionSemanticRejection:
		category = runtimecontract.RuntimeFeedbackCategorySemanticRejection
	case RuntimeStateUpdateDispositionOwnerTransient:
		category = runtimecontract.RuntimeFeedbackCategoryOwnerTransient
	default:
		return fmt.Errorf("%w: unsupported runtime state update disposition", common.ErrInvalidArgument)
	}
	if r.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainPersistence || r.Feedback.StableCategory != category ||
		r.Feedback.WorkspaceID != r.WorkspaceID || r.Feedback.RunID != r.RunID {
		return fmt.Errorf("%w: runtime state update feedback does not match disposition or scope", common.ErrInvalidArgument)
	}
	if err := r.Feedback.Validate(); err != nil {
		return fmt.Errorf("%w: runtime state update feedback: %v", common.ErrInvalidArgument, err)
	}
	return nil
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
	RunID              common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	State              common.RunState
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
	CausationID        string
	// RuntimeStateRevision and RuntimeStateRef are opaque SM-2/3/4 persistence
	// identity. A Run notification must not mirror convergence, action, focus,
	// repair, authority or dispatch semantics owned by Orchestrator.
	RuntimeStateRevision uint64
	RuntimeStateRef      string
	// RuntimeActivationDisposition is a Run-owned administrative outcome. It
	// does not expose or derive an Orchestrator action, focus, dispatch, or
	// recovery classification.
	RuntimeActivationDisposition runmodel.RuntimeActivationDisposition
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
		RuntimeStateRef:    "",
	}
	if run.RuntimeState != nil {
		ret.RuntimeStateRevision = run.RuntimeState.StateRevision
		ret.RuntimeStateRef = RuntimeStateRef(run.WorkspaceID, run.ID, run.RuntimeState.StateRevision)
	}
	if run.RuntimeActivationStatus != nil {
		ret.RuntimeActivationDisposition = run.RuntimeActivationStatus.Disposition
	}
	ret.CausationID = "run-state/" + string(run.ID) + "/" + fmt.Sprintf("%d", ret.RuntimeStateRevision)
	if ret.RuntimeActivationDisposition != "" {
		ret.CausationID += "/" + string(ret.RuntimeActivationDisposition)
	}
	return ret
}

// RuntimeStateRef returns the opaque Run-owner reference used by recovery,
// lifecycle facts, and schema observations. It contains no decoded aggregate
// state.
func RuntimeStateRef(workspaceID common.WorkspaceID, runID common.RunID, revision uint64) string {
	if workspaceID == "" || runID == "" {
		return ""
	}
	return fmt.Sprintf("runtime-state/%s/%s/%d", workspaceID, runID, revision)
}

// RequiresFreshActivation reports the only currently non-resumable Run-owned
// administrative disposition. It is a pure projection helper and never
// selects an Orchestrator drive action.
func (d RunStateChangedData) RequiresFreshActivation() bool {
	return d.RuntimeActivationDisposition == runmodel.RuntimeActivationDispositionFreshActivationRequired
}

// Validate verifies the immutable identity carried by a Run owner
// notification. A zero RuntimeStateRevision is permitted only for the
// pre-runtime create notification; every Runtime V2 transition carries the
// persisted authority/facts identity.
func (d RunStateChangedData) Validate() error {
	if d.WorkspaceID == "" || d.RunID == "" || d.CausationID == "" {
		return fmt.Errorf("%w: run state fact requires scope and causation", common.ErrInvalidArgument)
	}
	if d.RuntimeStateRevision > 0 && strings.TrimSpace(d.RuntimeStateRef) == "" {
		return fmt.Errorf("%w: runtime run state fact requires opaque runtime state ref", common.ErrInvalidArgument)
	}
	if d.RuntimeActivationDisposition != "" && d.RuntimeActivationDisposition != runmodel.RuntimeActivationDispositionFreshActivationRequired {
		return fmt.Errorf("%w: unsupported runtime activation disposition", common.ErrInvalidArgument)
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
	// RuntimeStateUpdate is populated only for an opaque Runtime V2 aggregate
	// update. It gives Orchestrator an exact persistence fact rather than an
	// ambiguous generic conflict.
	RuntimeStateUpdate *RuntimeStateUpdateResult
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
		eventcontract.CommandTopic[ApplyRuntimeStateIncompatibleFactCommand, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandApplyRuntimeStateIncompatible).MessageContract(),
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

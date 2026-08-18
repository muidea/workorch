package events

import (
	"fmt"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateRunCommand struct {
	RunID               common.RunID
	WorkspaceID         common.WorkspaceID
	GoalID              common.GoalID
	DoDID               common.DoDID
	DefinitionRevision  string
	InitialStepKind     string
	RuntimeBootstrap    *runtimecontract.RuntimeBootstrap
	PredecessorRunID    common.RunID
	PredecessorResultID common.ResultID
	Priority            int
}

type StoreRuntimeStateRecordCommand struct {
	RunID                        common.RunID
	WorkspaceID                  common.WorkspaceID
	RuntimeState                 *runtimecontract.RuntimeStateRecord
	LifecycleFence               *runtimecontract.RuntimeOpaqueLifecycleFence
	ExpectedRuntimeStateRevision *uint64
}

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

type QueryRuntimeRecoveryInventoryCommand struct {
	Cursor string
	Limit  int
}

type RuntimeRecoveryInventoryItem struct {
	WorkspaceID                  common.WorkspaceID
	RunID                        common.RunID
	State                        common.RunState
	HasRuntimeBootstrap          bool
	HasRuntimeState              bool
	RuntimeStateRef              string
	RuntimeStateRevision         uint64
	RuntimeStateSchemaVersion    uint32
	RuntimeActivationDisposition runtimecontract.RuntimeActivationDisposition
}

type QueryRuntimeRecoveryInventoryResult struct {
	Items      []RuntimeRecoveryInventoryItem
	NextCursor string
}

// QueryRuntimeStateRecordReadinessCommand asks whether Run's ordinary
// command surface is ready. It is a process lifecycle query, not a Runtime
// state transition and therefore has no workspace/run payload.
type QueryRuntimeStateRecordReadinessCommand struct{}

type QueryRuntimeStateRecordReadinessResult struct {
	Ready bool
}

// QueryRuntimeStateRecordCommand reads exactly one Run-owned opaque Runtime
// state record. During startup callers bind the inventory's opaque ref and
// revision; ordinary runtime reloads may omit both and receive the current
// record. Run never interprets the aggregate carried by this contract.
type QueryRuntimeStateRecordCommand struct {
	WorkspaceID                  common.WorkspaceID
	RunID                        common.RunID
	ExpectedRuntimeStateRef      string
	ExpectedRuntimeStateRevision uint64
}

func (c QueryRuntimeStateRecordCommand) Validate() error {
	if c.WorkspaceID == "" || c.RunID == "" {
		return fmt.Errorf("%w: runtime state record query requires scope", common.ErrInvalidArgument)
	}
	hasRef := strings.TrimSpace(c.ExpectedRuntimeStateRef) != ""
	hasRevision := c.ExpectedRuntimeStateRevision != 0
	if hasRef != hasRevision {
		return fmt.Errorf("%w: runtime state record query requires both opaque ref and revision", common.ErrInvalidArgument)
	}
	if hasRef && c.ExpectedRuntimeStateRef != RuntimeStateRef(c.WorkspaceID, c.RunID, c.ExpectedRuntimeStateRevision) {
		return fmt.Errorf("%w: runtime state record query opaque ref does not match scope", common.ErrInvalidArgument)
	}
	return nil
}

// QueryRuntimeStateRecordResult is an opaque persistence result. StatePresent
// distinguishes a missing record from a malformed response, and the returned
// record is cloned by the Run owner before leaving its repository boundary.
type QueryRuntimeStateRecordResult struct {
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	StatePresent         bool
	RuntimeStateRef      string
	RuntimeStateRevision uint64
	RuntimeState         *runtimecontract.RuntimeStateRecord
}

func (r QueryRuntimeStateRecordResult) Validate() error {
	if r.WorkspaceID == "" || r.RunID == "" {
		return fmt.Errorf("%w: runtime state record result requires scope", common.ErrInvalidArgument)
	}
	if !r.StatePresent {
		if r.RuntimeState != nil || r.RuntimeStateRef != "" || r.RuntimeStateRevision != 0 {
			return fmt.Errorf("%w: absent runtime state record contains state", common.ErrInvalidArgument)
		}
		return nil
	}
	if r.RuntimeState == nil || r.RuntimeStateRevision == 0 ||
		r.RuntimeState.StateRevision != r.RuntimeStateRevision ||
		r.RuntimeStateRef != RuntimeStateRef(r.WorkspaceID, r.RunID, r.RuntimeStateRevision) {
		return fmt.Errorf("%w: runtime state record result does not match opaque identity", common.ErrInvalidArgument)
	}
	return nil
}

// RuntimeStateUpdateResult is a concrete cross-owner fact. It preserves both
// aggregate revision fence values and the bounded producer diagnostic without
// carrying decoded aggregate authority.
type RuntimeStateUpdateResult struct {
	WorkspaceID             common.WorkspaceID
	RunID                   common.RunID
	ExpectedRuntimeRevision uint64
	ObservedRuntimeRevision uint64
	Feedback                runtimecontract.RuntimeFeedback
}

func (r RuntimeStateUpdateResult) Validate() error {
	if r.WorkspaceID == "" || r.RunID == "" {
		return fmt.Errorf("%w: runtime state update result requires scope", common.ErrInvalidArgument)
	}
	switch r.Feedback.StableCategory {
	case runtimecontract.RuntimeFeedbackCategoryAccepted,
		runtimecontract.RuntimeFeedbackCategoryAggregateRevisionMismatch,
		runtimecontract.RuntimeFeedbackCategorySemanticRejection,
		runtimecontract.RuntimeFeedbackCategoryOwnerTransient:
	default:
		return fmt.Errorf("%w: unsupported runtime state update outcome", common.ErrInvalidArgument)
	}
	if r.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainPersistence ||
		r.Feedback.WorkspaceID != r.WorkspaceID || r.Feedback.RunID != r.RunID {
		return fmt.Errorf("%w: runtime state update feedback does not match scope", common.ErrInvalidArgument)
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

type ListStepSummariesByRunCommand ListStepsByRunCommand

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
	RuntimeActivationDisposition runtimecontract.RuntimeActivationDisposition
	// RuntimeStateIncompatibility is emitted only with a Run-owned
	// fresh-activation-required fact. It identifies an opaque persisted record
	// without exposing aggregate authority.
	RuntimeStateIncompatibility runtimecontract.RuntimeStateIncompatibility
	// ReconciliationRequestID is present only when this active transition was
	// accepted for a pending Run resume. It is an immutable correlation fact,
	// not a requested lifecycle target.
	ReconciliationRequestID string
}

// RunContinuationRequestedData is emitted only by Run owner after it accepts
// a scoped Continue intent for an already-active Run. It deliberately differs
// from RunStateChangedData: duplicate resume is not a fresh SM-1 transition.
type RunContinuationRequestedData struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	CausationID string
	// RequestID is the idempotency identity of this accepted continuation.
	// It is distinct from the Run lifecycle state: duplicate resume with the
	// same request reuses the same reconciliation generation, while a new
	// request forces a fresh current-workspace observation.
	RequestID string
}

// RuntimeCommandsReadyData is emitted once per Run owner Start after all
// regular Run command subscriptions are live. It has no workspace/run scope
// because it represents process readiness rather than a workspace state fact.
type RuntimeCommandsReadyData struct {
	CausationID string
}

func (d RuntimeCommandsReadyData) Validate() error {
	if strings.TrimSpace(d.CausationID) == "" {
		return fmt.Errorf("%w: runtime commands ready fact requires causation", common.ErrInvalidArgument)
	}
	return nil
}

func (d RunContinuationRequestedData) Validate() error {
	d = d.Normalized()
	if d.WorkspaceID == "" || d.RunID == "" || d.CausationID == "" || d.RequestID == "" {
		return fmt.Errorf("%w: run continuation fact requires scope and causation", common.ErrInvalidArgument)
	}
	return nil
}

func (d RunContinuationRequestedData) Normalized() RunContinuationRequestedData {
	d.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(d.WorkspaceID)))
	d.RunID = common.RunID(strings.TrimSpace(string(d.RunID)))
	d.CausationID = strings.TrimSpace(d.CausationID)
	d.RequestID = strings.TrimSpace(d.RequestID)
	if d.RequestID == "" && d.CausationID != "" {
		d.RequestID = "continuation/" + d.CausationID
	}
	return d
}

func RunStateChangedDataFromModel(run *runmodel.Run) RunStateChangedData {
	if run == nil {
		return RunStateChangedData{}
	}
	ret := RunStateChangedData{
		RunID:                   run.ID,
		WorkspaceID:             run.WorkspaceID,
		GoalID:                  run.GoalID,
		DoDID:                   run.DoDID,
		State:                   run.State,
		BlockingTicketID:        run.BlockingTicketID,
		BlockingApprovalID:      run.BlockingApprovalID,
		FailureReason:           run.FailureReason,
		RuntimeStateRef:         "",
		ReconciliationRequestID: strings.TrimSpace(run.ReconciliationRequestID),
	}
	if run.RuntimeState != nil {
		ret.RuntimeStateRevision = run.RuntimeState.StateRevision
		ret.RuntimeStateRef = RuntimeStateRef(run.WorkspaceID, run.ID, run.RuntimeState.StateRevision)
	}
	if run.RuntimeActivationStatus != nil {
		ret.RuntimeActivationDisposition = run.RuntimeActivationStatus.Disposition
		ret.RuntimeStateIncompatibility = run.RuntimeActivationStatus.Incompatibility.Normalized()
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
	return d.RuntimeActivationDisposition == runtimecontract.RuntimeActivationDispositionFreshActivationRequired
}

// Validate verifies the immutable identity carried by a Run owner
// notification. A zero RuntimeStateRevision is permitted only for the
// pre-runtime create notification; every Runtime transition carries the
// persisted authority/facts identity.
func (d RunStateChangedData) Validate() error {
	if d.WorkspaceID == "" || d.RunID == "" || d.CausationID == "" {
		return fmt.Errorf("%w: run state fact requires scope and causation", common.ErrInvalidArgument)
	}
	if d.RuntimeStateRevision > 0 && strings.TrimSpace(d.RuntimeStateRef) == "" {
		return fmt.Errorf("%w: runtime run state fact requires opaque runtime state ref", common.ErrInvalidArgument)
	}
	if d.RuntimeActivationDisposition != "" && d.RuntimeActivationDisposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
		return fmt.Errorf("%w: unsupported runtime activation disposition", common.ErrInvalidArgument)
	}
	if d.RuntimeActivationDisposition == runtimecontract.RuntimeActivationDispositionFreshActivationRequired &&
		(d.RuntimeStateRevision == 0 || strings.TrimSpace(d.RuntimeStateRef) == "" ||
			d.RuntimeStateIncompatibility.Validate() != nil) {
		return fmt.Errorf("%w: fresh activation run state fact requires incompatible persisted state identity", common.ErrInvalidArgument)
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
	// RuntimeStateUpdate is populated only for an opaque Runtime aggregate
	// update. It gives Orchestrator an exact persistence fact rather than an
	// ambiguous generic conflict.
	RuntimeStateUpdate *RuntimeStateUpdateResult
}

func (r RunCommandResult) Clone() RunCommandResult {
	cloned := r
	cloned.Run = runmodel.CloneRun(r.Run)
	cloned.Runs = runmodel.CloneRuns(r.Runs)
	cloned.Step = runmodel.CloneStep(r.Step)
	cloned.Steps = runmodel.CloneSteps(r.Steps)
	cloned.Checkpoint = runmodel.CloneRuntimeCheckpoint(r.Checkpoint)
	cloned.Checkpoints = runmodel.CloneRuntimeCheckpoints(r.Checkpoints)
	if r.RuntimeStateUpdate != nil {
		update := *r.RuntimeStateUpdate
		update.Feedback = r.RuntimeStateUpdate.Feedback.CloneOriginal()
		cloned.RuntimeStateUpdate = &update
	}
	return cloned
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[GetRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListRunsCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandList).MessageContract(),
		eventcontract.CommandTopic[ListRunsByWorkspaceCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeRecoveryInventoryCommand, QueryRuntimeRecoveryInventoryResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandRuntimeRecoveryInventory).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeStateRecordReadinessCommand, QueryRuntimeStateRecordReadinessResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandRuntimeStateRecordReadiness).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeStateRecordCommand, QueryRuntimeStateRecordResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandRuntimeStateRecord).MessageContract(),
		eventcontract.CommandTopic[GetStepCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGetStep).MessageContract(),
		eventcontract.CommandTopic[ListStepsByRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListStepsByRun).MessageContract(),
		eventcontract.CommandTopic[ListStepSummariesByRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListStepSummariesByRun).MessageContract(),
		eventcontract.CommandTopic[GetRuntimeCheckpointCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGetRuntimeCheckpoint).MessageContract(),
		eventcontract.CommandTopic[ListRuntimeCheckpointsByRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListCheckpointsByRun).MessageContract(),
		eventcontract.CommandTopic[PurgeRuntimeResourcesCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandPurgeRuntimeResources).MessageContract(),
		eventcontract.CommandTopic[StoreRuntimeStateRecordCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandStoreRuntimeStateRecord).MessageContract(),
		eventcontract.CommandTopic[DeleteRunCommand, RunCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandDelete).MessageContract(),
		eventcontract.CommandTopic[WorkspaceExecutionIntentCommand, WorkspaceExecutionIntentResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandWorkspaceExecutionIntent).MessageContract(),
		eventcontract.CommandTopic[RuntimeGovernanceWaitFact, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunFactGovernanceWait).MessageContract(),
		eventcontract.CommandTopic[RuntimeContinuationEligibleFact, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunFactContinuationEligible).MessageContract(),
		eventcontract.CommandTopic[RuntimeTerminalDeliveryFact, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunFactTerminalDelivery).MessageContract(),
		eventcontract.CommandTopic[RuntimeStateIncompatibleFact, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunFactRuntimeStateIncompatible).MessageContract(),
	}
	for _, topic := range []string{
		TopicRunCreated,
		TopicRunStarted,
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
	ret = append(ret, eventcontract.DataTopic[RuntimeCommandsReadyData]("run", definitionPackage, eventcontract.LanePolicyState, TopicRunRuntimeCommandsReady).MessageContract())
	return ret
}

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

type StoreRuntimeStateRecordCommand struct {
	RunID                        common.RunID
	WorkspaceID                  common.WorkspaceID
	RuntimeState                 *runtimecontract.RuntimeStateRecord
	LifecycleFence               *runtimecontract.RuntimeOpaqueLifecycleFence
	ExpectedRuntimeStateRevision *uint64
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

// QueryRunLifecycleFactInventoryCommand asks the Run owner for a bounded page
// of current non-terminal lifecycle facts. It is a read/reconciliation
// contract, not an instruction to choose a Run or Workspace state.
type QueryRunLifecycleFactInventoryCommand struct {
	Cursor string
	Limit  int
}

type QueryRunLifecycleFactInventoryResult struct {
	Items      []RunStateChangedData
	NextCursor string
}

func (r QueryRunLifecycleFactInventoryResult) Validate() error {
	if len(r.Items) > 500 {
		return fmt.Errorf("%w: run lifecycle fact inventory exceeds page limit", common.ErrInvalidArgument)
	}
	previous := ""
	for idx, item := range r.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("%w: run lifecycle fact inventory item %d: %v", common.ErrInvalidArgument, idx, err)
		}
		switch item.State {
		case common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		default:
			return fmt.Errorf("%w: run lifecycle fact inventory contains terminal or unsupported state", common.ErrInvalidArgument)
		}
		key := RunLifecycleFactInventoryCursorKey(item)
		if previous != "" && key <= previous {
			return fmt.Errorf("%w: run lifecycle fact inventory is not strictly ordered", common.ErrInvalidArgument)
		}
		previous = key
	}
	if r.NextCursor != "" && (len(r.Items) == 0 || r.NextCursor != previous) {
		return fmt.Errorf("%w: run lifecycle fact inventory cursor does not match page tail", common.ErrInvalidArgument)
	}
	return nil
}

type QueryRuntimeRecoveryInventoryCommand struct {
	Cursor string
	Limit  int
}

type RuntimeRecoveryInventoryItem struct {
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	State                common.RunState
	HasRuntimeBootstrap  bool
	HasRuntimeState      bool
	RuntimeStateRef      string
	RuntimeStateRevision uint64
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

type RunStateChangedData struct {
	RunID       common.RunID
	WorkspaceID common.WorkspaceID
	// WorkspaceRunGeneration is the Run owner's durable Workspace-scoped
	// lineage coordinate. It is allocated at Run creation and never inferred
	// from RunID, timestamps, notification order, or Runtime state.
	WorkspaceRunGeneration uint64
	// RunRevision is the Run owner's lifecycle/model CAS revision. Together
	// with RuntimeStateRevision it gives consumers a monotonic factual fence
	// without exposing or transferring SM-1 or Runtime authority.
	RunRevision        uint64
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
		WorkspaceRunGeneration:  run.WorkspaceRunGeneration,
		RunRevision:             run.Revision,
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
	ret.CausationID = RunStateChangedCausationID(run.ID, ret.WorkspaceRunGeneration, ret.RunRevision, ret.RuntimeStateRevision)
	return ret
}

// RunStateChangedCausationID is derived only from Run-owner persisted
// revisions. A lifecycle transition with the same Runtime revision still has
// a distinct identity because the Run model revision advances.
func RunStateChangedCausationID(runID common.RunID, workspaceRunGeneration, runRevision, runtimeStateRevision uint64) string {
	if runID == "" || workspaceRunGeneration == 0 || runRevision == 0 {
		return ""
	}
	return fmt.Sprintf("run-state/%s/%d/%d/%d", runID, workspaceRunGeneration, runRevision, runtimeStateRevision)
}

// RunLifecycleFactInventoryCursorKey is the Run-owner opaque current-fact
// order. Generation precedes RunID so pagination never infers lifecycle order
// from an identifier's spelling.
func RunLifecycleFactInventoryCursorKey(d RunStateChangedData) string {
	return fmt.Sprintf("%s\x00%020d\x00%s", d.WorkspaceID, d.WorkspaceRunGeneration, d.RunID)
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

// Validate verifies the immutable identity carried by a Run owner
// notification. A zero RuntimeStateRevision is permitted only for the
// pre-runtime create notification; every Runtime transition carries the
// persisted authority/facts identity.
func (d RunStateChangedData) Validate() error {
	if d.WorkspaceID == "" || d.RunID == "" || d.WorkspaceRunGeneration == 0 || d.RunRevision == 0 || d.CausationID == "" {
		return fmt.Errorf("%w: run state fact requires scope and causation", common.ErrInvalidArgument)
	}
	if d.CausationID != RunStateChangedCausationID(d.RunID, d.WorkspaceRunGeneration, d.RunRevision, d.RuntimeStateRevision) {
		return fmt.Errorf("%w: run state fact causation does not match owner revisions", common.ErrInvalidArgument)
	}
	if d.RuntimeStateRevision > 0 && strings.TrimSpace(d.RuntimeStateRef) == "" {
		return fmt.Errorf("%w: runtime run state fact requires opaque runtime state ref", common.ErrInvalidArgument)
	}
	if d.RuntimeStateRevision == 0 && strings.TrimSpace(d.RuntimeStateRef) != "" {
		return fmt.Errorf("%w: pre-runtime run state fact must not carry opaque runtime state ref", common.ErrInvalidArgument)
	}
	if d.RuntimeStateRevision > 0 && d.RuntimeStateRef != RuntimeStateRef(d.WorkspaceID, d.RunID, d.RuntimeStateRevision) {
		return fmt.Errorf("%w: runtime run state fact opaque ref does not match scope", common.ErrInvalidArgument)
	}
	return nil
}

// The Run owner deliberately exposes one result shape per command. Keeping
// these results distinct prevents an unrelated topic from being accepted just
// because it populated another field in a shared union envelope.
type GetRunResult struct{ Run *runmodel.Run }
type ListRunsResult struct{ Runs []*runmodel.Run }
type ListRunsByWorkspaceResult struct{ Runs []*runmodel.Run }
type GetStepResult struct{ Step *runmodel.Step }
type ListStepsByRunResult struct{ Steps []*runmodel.Step }
type ListStepSummariesByRunResult struct{ Steps []*runmodel.Step }
type GetRuntimeCheckpointResult struct{ Checkpoint *runmodel.RuntimeCheckpoint }
type ListRuntimeCheckpointsByRunResult struct{ Checkpoints []*runmodel.RuntimeCheckpoint }

// StoreRuntimeStateRecordResult is the concrete opaque-CAS receipt. It exposes
// only the canonical persistence feedback consumed by Orchestrator, not a
// second full Run snapshot.
type StoreRuntimeStateRecordResult struct {
	RuntimeStateUpdate *RuntimeStateUpdateResult
}

func (r GetRunResult) Clone() GetRunResult {
	return GetRunResult{Run: runmodel.CloneRun(r.Run)}
}

func (r ListRunsResult) Clone() ListRunsResult {
	return ListRunsResult{Runs: runmodel.CloneRuns(r.Runs)}
}

func (r ListRunsByWorkspaceResult) Clone() ListRunsByWorkspaceResult {
	return ListRunsByWorkspaceResult{Runs: runmodel.CloneRuns(r.Runs)}
}

func (r GetStepResult) Clone() GetStepResult {
	return GetStepResult{Step: runmodel.CloneStep(r.Step)}
}

func (r ListStepsByRunResult) Clone() ListStepsByRunResult {
	return ListStepsByRunResult{Steps: runmodel.CloneSteps(r.Steps)}
}

func (r ListStepSummariesByRunResult) Clone() ListStepSummariesByRunResult {
	return ListStepSummariesByRunResult{Steps: runmodel.CloneSteps(r.Steps)}
}

func (r GetRuntimeCheckpointResult) Clone() GetRuntimeCheckpointResult {
	return GetRuntimeCheckpointResult{Checkpoint: runmodel.CloneRuntimeCheckpoint(r.Checkpoint)}
}

func (r ListRuntimeCheckpointsByRunResult) Clone() ListRuntimeCheckpointsByRunResult {
	return ListRuntimeCheckpointsByRunResult{Checkpoints: runmodel.CloneRuntimeCheckpoints(r.Checkpoints)}
}

func (r StoreRuntimeStateRecordResult) Clone() StoreRuntimeStateRecordResult {
	cloned := StoreRuntimeStateRecordResult{}
	if r.RuntimeStateUpdate != nil {
		update := *r.RuntimeStateUpdate
		update.Feedback = r.RuntimeStateUpdate.Feedback.CloneOriginal()
		cloned.RuntimeStateUpdate = &update
	}
	return cloned
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[GetRunCommand, GetRunResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGet).MessageContract(),
		eventcontract.CommandTopic[ListRunsCommand, ListRunsResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandList).MessageContract(),
		eventcontract.CommandTopic[ListRunsByWorkspaceCommand, ListRunsByWorkspaceResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListByWorkspace).MessageContract(),
		eventcontract.CommandTopic[QueryRunLifecycleFactInventoryCommand, QueryRunLifecycleFactInventoryResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandLifecycleFactInventory).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeRecoveryInventoryCommand, QueryRuntimeRecoveryInventoryResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandRuntimeRecoveryInventory).MessageContract(),
		eventcontract.CommandTopic[QueryWorkspaceRetirementFactCommand, QueryWorkspaceRetirementFactResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandWorkspaceRetirementFact).MessageContract(),
		eventcontract.CommandTopic[QueryWorkspaceRetirementInventoryCommand, QueryWorkspaceRetirementInventoryResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandWorkspaceRetirementInventory).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeStateRecordReadinessCommand, QueryRuntimeStateRecordReadinessResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandRuntimeStateRecordReadiness).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeStateRecordCommand, QueryRuntimeStateRecordResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandRuntimeStateRecord).MessageContract(),
		eventcontract.CommandTopic[GetStepCommand, GetStepResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGetStep).MessageContract(),
		eventcontract.CommandTopic[ListStepsByRunCommand, ListStepsByRunResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListStepsByRun).MessageContract(),
		eventcontract.CommandTopic[ListStepSummariesByRunCommand, ListStepSummariesByRunResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListStepSummariesByRun).MessageContract(),
		eventcontract.CommandTopic[GetRuntimeCheckpointCommand, GetRuntimeCheckpointResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandGetRuntimeCheckpoint).MessageContract(),
		eventcontract.CommandTopic[ListRuntimeCheckpointsByRunCommand, ListRuntimeCheckpointsByRunResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicRunCommandListCheckpointsByRun).MessageContract(),
		eventcontract.CommandTopic[StoreRuntimeStateRecordCommand, StoreRuntimeStateRecordResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandStoreRuntimeStateRecord).MessageContract(),
		eventcontract.CommandTopic[PrepareWorkspaceRetirementCommand, PrepareWorkspaceRetirementResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandPrepareWorkspaceRetirement).MessageContract(),
		eventcontract.CommandTopic[FinalizeWorkspaceRetirementCommand, FinalizeWorkspaceRetirementResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandFinalizeWorkspaceRetirement).MessageContract(),
		eventcontract.CommandTopic[PurgeWorkspaceRunsCommand, PurgeWorkspaceRunsResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandPurgeWorkspace).MessageContract(),
		eventcontract.CommandTopic[WorkspaceExecutionIntentCommand, WorkspaceExecutionIntentResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunCommandWorkspaceExecutionIntent).MessageContract(),
		eventcontract.CommandTopic[ObserveWorkspacePreparationHistoryCommand, ObserveWorkspacePreparationHistoryResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicObserveWorkspacePreparationHistory).MessageContract(),
		eventcontract.CommandTopic[QueryWorkspaceExecutionIntentReceiptCommand, QueryWorkspaceExecutionIntentReceiptResult]("run", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicQueryWorkspaceExecutionIntentReceipt).MessageContract(),
		eventcontract.CommandTopic[RuntimeGovernanceWaitFact, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunFactGovernanceWait).MessageContract(),
		eventcontract.CommandTopic[RuntimeContinuationEligibleFact, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunFactContinuationEligible).MessageContract(),
		eventcontract.CommandTopic[RuntimeTerminalDeliveryFact, RunLifecycleCommandResult]("run", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicRunFactTerminalDelivery).MessageContract(),
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

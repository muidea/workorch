package orchestrator

// Owner lifecycle and Runtime authority implementation.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	CommandHub            magicevent.Hub
	BackgroundRoutine     task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	runtimeController *runtime.RuntimeController
	evidence          RuntimeEvidenceRepositories
	evidenceRecords   repository.EvidenceRecordRepository
	rawTopics         []string
	// startupRecoveryActions are owner-local lifecycle handoff data only. They
	// are rebuilt from the persisted Run inventory on every process start and
	// are released solely by the Run owner's commands-ready fact; they are not
	// a second persisted Runtime state machine.
	startupRecoveryMu       sync.Mutex
	runRuntimeCommandsReady bool
	startupRecoveryActions  []runtimeStartupRecoveryAction
	// lifecycleMu protects Runtime owner dependencies while a background
	// drive is using them. Stop takes the write lock before clearing pointers;
	// every drive holds the read lock through authority and projection work.
	lifecycleMu sync.RWMutex
	driveMu     sync.Mutex
	drivingRuns map[string]struct{}
	// retryLeaseTimers is an in-process dedupe for an already persisted SM-4
	// lease. It is not retry authority and is discarded during process restart.
	retryLeaseTimers map[string]uint64
	started          bool
}

type runtimeStartupRecoveryActionKind string

const (
	runtimeStartupRecoveryActionDrive        runtimeStartupRecoveryActionKind = "drive"
	runtimeStartupRecoveryActionRecoverLease runtimeStartupRecoveryActionKind = "recover_lease"
	runtimeStartupRecoveryActionIncompatible runtimeStartupRecoveryActionKind = "incompatible"
)

type runtimeStartupRecoveryAction struct {
	Kind            runtimeStartupRecoveryActionKind
	Item            runevents.RuntimeRecoveryInventoryItem
	Source          string
	Incompatibility runtimecontract.RuntimeStateIncompatibility
	Diagnostic      error
}

func New() *Biz { return &Biz{} }

func (m *Biz) ID() string { return ModuleID }

func (m *Biz) Configure(deps Dependencies) error {
	if deps.ToolCalls == nil ||
		deps.Patches == nil ||
		deps.ValidationResults == nil ||
		deps.EvidenceRecords == nil ||
		deps.CoverageWorkItems == nil ||
		deps.ArtifactContinuations == nil ||
		deps.CommandHub == nil {
		return common.ErrInvalidArgument
	}
	m.Base = basebiz.New(ModuleID, deps.CommandHub, deps.BackgroundRoutine)
	m.evidence = RuntimeEvidenceRepositories{
		Results:               resultEventResultRepository{hub: m.EventHub()},
		Artifacts:             resultEventArtifactRepository{hub: deps.CommandHub},
		ToolCalls:             deps.ToolCalls,
		Patches:               deps.Patches,
		ValidationResults:     deps.ValidationResults,
		EvidenceRecords:       deps.EvidenceRecords,
		CoverageWorkItems:     deps.CoverageWorkItems,
		ArtifactContinuations: deps.ArtifactContinuations,
		MemoryItems:           memoryEventRepository{hub: m.EventHub()},
		MemoryRecallTraces:    memoryRecallTraceEventRepository{hub: m.EventHub()},
	}
	controller, err := runtime.NewRuntimeController(runtimeStateRepository{hub: m.EventHub()})
	if err != nil {
		return err
	}
	m.runtimeController = controller
	m.evidenceRecords = deps.EvidenceRecords
	return nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeController == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	if len(m.rawTopics) == 0 {
		m.rawTopics = []string{
			// RUNTIME-SMO: Run lifecycle topics are owned by blocks/run. The
			orchestratorevents.TopicOrchestratorCommandPurgeRun,
			orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
			orchestratorevents.TopicOrchestratorCommandListPatchesByRun,
			orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun,
			orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
			orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun,
		}
		for _, topic := range m.rawTopics {
			m.SubscribeFunc(topic, m.handleMagicCommand)
		}
		m.rawTopics = append(m.rawTopics,
			orchestratorevents.TopicOrchestratorDriveRuntime,
			orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
			orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt,
			orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
			orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
			orchestratorevents.TopicOrchestratorRequestRuntimeTermination,
			decisionevents.TopicDecisionCommandRecordEvidence,
			approvalevents.TopicApprovalCommandRecordEvidence,
		)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorDriveRuntime, m.handleRuntimeDrive)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, m.handleRuntimeOperationReceipt)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, m.handleRuntimeOperationReceipt)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution, m.handleRuntimeGovernanceResolution)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, m.handleRuntimeGovernanceTicketResolution)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, m.handleRuntimeTerminationRequest)
		m.SubscribeFunc(decisionevents.TopicDecisionCommandRecordEvidence, m.handleDecisionEvidenceCommand)
		m.SubscribeFunc(approvalevents.TopicApprovalCommandRecordEvidence, m.handleApprovalEvidenceCommand)
		m.SubscribeMatchedFunc(ModuleID+"/run-drive", runevents.NotificationDestination, runevents.TopicRunStateChanged, m.handleRunStateChanged)
		m.SubscribeMatchedFunc(ModuleID+"/run-drive", runevents.NotificationDestination, runevents.TopicRunContinuationRequested, m.handleRunContinuationRequested)
		m.SubscribeMatchedFunc(ModuleID+"/run-drive", runevents.NotificationDestination, runevents.TopicRunRuntimeCommandsReady, m.handleRunRuntimeCommandsReady)
	}
	if m.started {
		return nil
	}
	// GMM-R1 / SM-4: recovery inventory must be acquired synchronously. A missing
	// Run owner subscriber is a startup failure, not a warning that loses recovery.
	if err := m.recoverRuntimeStartup(context.Background()); err != nil {
		return err
	}
	m.started = true
	return nil
}

func (m *Biz) handleRuntimeTerminationRequest(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(orchestratorevents.RequestRuntimeTerminationCommand)
	// A termination request is only the persisted supersede fact emitted by
	// the Run owner.  No application/UI/governance caller may turn a command
	// into an SM-4 transition directly.
	if !ok || ev.Source() != "blocks/run" || m == nil || m.runtimeController == nil || cmd.Validate() != nil || cmd.RequestedBy != "blocks/run" {
		setOrchestratorAck(result, common.ErrInvalidArgument)
		return
	}
	terminated, err := m.submitRuntimeInternalEvent(ev.Context(), runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventTerminationRequested, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID,
		TerminationKind: cmd.Kind, RequestedBy: cmd.RequestedBy, Reason: cmd.Reason,
	})
	if err != nil {
		setOrchestratorAck(result, err)
		return
	}
	aggregate := terminated.Aggregate
	request := terminalDeliveryRequest(aggregate)
	err = m.deliverRuntimeTerminal(ev.Context(), aggregate)
	if err != nil {
		setOrchestratorAck(result, err)
		return
	}
	completed, err := m.runtimeController.Load(ev.Context(), cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		setOrchestratorAck(result, err)
		return
	}
	if result != nil {
		result.Set(orchestratorevents.RequestRuntimeTerminationResult{
			TerminalDeliveryID: request.TerminalDeliveryID,
			WorkspaceReleased:  deliveryAcknowledged(completed, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerWorkspace),
			StateRevision:      completed.StateRevision,
		}, nil)
	}
}

func (m *Biz) recoverRuntimeStartup(ctx context.Context) error {
	if m == nil || m.runtimeController == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	items, err := queryRuntimeRecoveryInventory(ctx, m.EventHub())
	if err != nil {
		return fmt.Errorf("Runtime startup recovery inventory: %w", err)
	}
	actions := make([]runtimeStartupRecoveryAction, 0)
	for _, item := range items {
		// A Run-owned administrative outcome is not an Orchestrator failure and
		// must never re-enter SM-4. Only explicit fresh activation creates a new
		// current-schema Run.
		if item.RuntimeActivationDisposition == runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
			continue
		}
		if !item.HasRuntimeState {
			if item.State == common.RunStateActive && item.HasRuntimeBootstrap {
				actions = append(actions, runtimeStartupRecoveryAction{Kind: runtimeStartupRecoveryActionDrive, Item: item, Source: "startup_recovery:bootstrap"})
			}
			continue
		}

		aggregate, loadErr := (runtimeStateRepository{hub: m.EventHub()}).LoadRuntimeRecoveryRecord(ctx, item)
		if loadErr != nil {
			if incompatibility, incompatible := runtimecontract.RuntimeStateIncompatibilityFromError(loadErr); incompatible {
				actions = append(actions, runtimeStartupRecoveryAction{Kind: runtimeStartupRecoveryActionIncompatible, Item: item, Source: "startup_recovery:state_incompatible", Incompatibility: incompatibility, Diagnostic: loadErr})
				continue
			}
			return fmt.Errorf("Runtime startup recovery load workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, loadErr)
		}

		// A persisted SM-4 lease survives process restart. It is resumed before
		// any normal classification so startup cannot create a parallel drive.
		if aggregate.DriveLease != nil {
			actions = append(actions, runtimeStartupRecoveryAction{Kind: runtimeStartupRecoveryActionRecoverLease, Item: item, Source: "startup_recovery"})
			continue
		}

		// Recovery classification is exclusively an Orchestrator SM-4 concern
		// after typed aggregate load. The Run inventory remains opaque.
		source := ""
		switch {
		case aggregate.Decision.State == runtimecontract.ConvergenceStateComplete || aggregate.Decision.State == runtimecontract.ConvergenceStateFailed:
			source = "startup_recovery:terminal_delivery"
		case aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance:
			// The current Run lifecycle is blocked on its persisted governance
			// binding. It is not drive eligible until the owner receives a
			// continuation fact.
			continue
		case item.State == common.RunStateActive:
			source = "startup_recovery:active"
		default:
			continue
		}
		actions = append(actions, runtimeStartupRecoveryAction{Kind: runtimeStartupRecoveryActionDrive, Item: item, Source: source})
	}
	if err := m.queueRuntimeStartupRecoveryActions(ctx, actions); err != nil {
		return fmt.Errorf("Runtime startup recovery handoff: %w", err)
	}
	slog.Info("workorch Runtime startup recovery inventory complete", "prepared", len(actions))
	return nil
}

// queueRuntimeStartupRecoveryActions explicitly hands already-loaded SM-4
// recovery work to the Run owner readiness boundary. This avoids relying on
// plugin Weight() and prevents the background routine from sending normal Run
// update commands before Run.Start has registered them.
func (m *Biz) queueRuntimeStartupRecoveryActions(ctx context.Context, actions []runtimeStartupRecoveryAction) error {
	if m == nil || len(actions) == 0 {
		return nil
	}
	ready, err := queryRuntimeStateRecordReadiness(ctx, m.EventHub())
	if err != nil {
		return err
	}
	m.startupRecoveryMu.Lock()
	if ready || m.runRuntimeCommandsReady {
		m.runRuntimeCommandsReady = true
		m.startupRecoveryMu.Unlock()
		return m.releaseRuntimeStartupRecoveryActions(ctx, actions)
	}
	m.startupRecoveryActions = append(m.startupRecoveryActions, actions...)
	m.startupRecoveryMu.Unlock()
	return nil
}

func (m *Biz) handleRunRuntimeCommandsReady(ev magicevent.Event, _ magicevent.Result) {
	data, ok := ev.Data().(runevents.RuntimeCommandsReadyData)
	if !ok || m == nil || ev.Source() != runevents.CommandDestination || data.Validate() != nil {
		return
	}
	// The ready fact can be queued while framework teardown begins. Keep the
	// owner dependencies alive while releasing the handoff; Stop takes the
	// matching write lock before clearing the controller.
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	if m.runtimeController == nil {
		return
	}
	m.startupRecoveryMu.Lock()
	m.runRuntimeCommandsReady = true
	actions := append([]runtimeStartupRecoveryAction(nil), m.startupRecoveryActions...)
	m.startupRecoveryActions = nil
	m.startupRecoveryMu.Unlock()
	if err := m.releaseRuntimeStartupRecoveryActions(ev.Context(), actions); err != nil {
		slog.Error("workorch Runtime startup recovery handoff failed after Run commands became ready", "err", err, "causationID", data.CausationID)
	}
}

func (m *Biz) releaseRuntimeStartupRecoveryActions(ctx context.Context, actions []runtimeStartupRecoveryAction) error {
	for _, action := range actions {
		item := action.Item
		switch action.Kind {
		case runtimeStartupRecoveryActionDrive:
			if err := m.scheduleRuntimeDrive(ctx, item.WorkspaceID, item.RunID, action.Source); err != nil {
				return fmt.Errorf("schedule workspace=%s run=%s source=%s: %w", item.WorkspaceID, item.RunID, action.Source, err)
			}
		case runtimeStartupRecoveryActionRecoverLease:
			aggregate, err := m.runtimeController.Load(ctx, item.WorkspaceID, item.RunID)
			if err != nil {
				return fmt.Errorf("reload lease workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, err)
			}
			if aggregate.DriveLease == nil {
				if err := m.scheduleRuntimeDrive(ctx, item.WorkspaceID, item.RunID, action.Source+":lease_reloaded"); err != nil {
					return fmt.Errorf("schedule reloaded lease workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, err)
				}
				continue
			}
			lease, err := m.recoverRuntimeDriveLease(ctx, aggregate, action.Source)
			if err != nil {
				return fmt.Errorf("recover lease workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, err)
			}
			key := runtimeDriveKey(item.WorkspaceID, item.RunID)
			if m.beginRuntimeDrive(key) && !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, action.Source) {
				m.endRuntimeDrive(key)
				return fmt.Errorf("lease timer workspace=%s run=%s was not accepted", item.WorkspaceID, item.RunID)
			}
		case runtimeStartupRecoveryActionIncompatible:
			if err := m.applyRunRuntimeStateIncompatible(ctx, item.WorkspaceID, item.RunID, item.RuntimeStateRef, action.Incompatibility, action.Diagnostic); err != nil {
				return fmt.Errorf("runtime state incompatibility workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, err)
			}
		default:
			return common.ErrInvalidArgument
		}
	}
	return nil
}

func queryRuntimeStateRecordReadiness(ctx context.Context, hub magicevent.Hub) (bool, error) {
	if hub == nil {
		return false, common.ErrInvalidArgument
	}
	result := hub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunCommandRuntimeStateRecordReadiness,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		runevents.QueryRuntimeStateRecordReadinessCommand{},
	))
	if result == nil {
		return false, common.ErrInvalidArgument
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return false, resultErr
	}
	response, ok := data.(runevents.QueryRuntimeStateRecordReadinessResult)
	if !ok {
		return false, common.ErrInvalidArgument
	}
	return response.Ready, nil
}

func queryRuntimeRecoveryInventory(ctx context.Context, hub magicevent.Hub) ([]runevents.RuntimeRecoveryInventoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]runevents.RuntimeRecoveryInventoryItem, 0)
	cursor := ""
	for {
		result := hub.Send(magicevent.NewEventWithContext(
			runevents.TopicRunCommandRuntimeRecoveryInventory, ModuleID, runevents.CommandDestination,
			magicevent.NewHeader(), ctx, runevents.QueryRuntimeRecoveryInventoryCommand{Cursor: cursor, Limit: 100},
		))
		if result == nil {
			return nil, common.ErrInvalidArgument
		}
		data, magicErr := result.Get()
		if magicErr != nil {
			return nil, magicErr
		}
		page, ok := data.(runevents.QueryRuntimeRecoveryInventoryResult)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		ret = append(ret, page.Items...)
		if strings.TrimSpace(page.NextCursor) == "" {
			return ret, nil
		}
		if page.NextCursor == cursor {
			return nil, common.ErrConflict
		}
		cursor = page.NextCursor
	}
}

func (m *Biz) Stop(ctx context.Context) error {
	_ = ctx
	if m == nil {
		return nil
	}
	// A drive can still be executing after its EventHub subscription has been
	// removed. Wait for it before dropping the owner dependencies it uses; this
	// is lifecycle synchronization only, not a second recovery state machine.
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	for idx := len(m.rawTopics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.rawTopics[idx])
	}
	m.UnsubscribeMatchedFunc(ModuleID+"/run-drive", runevents.TopicRunStateChanged)
	m.UnsubscribeMatchedFunc(ModuleID+"/run-drive", runevents.TopicRunContinuationRequested)
	m.UnsubscribeMatchedFunc(ModuleID+"/run-drive", runevents.TopicRunRuntimeCommandsReady)
	m.rawTopics = nil
	m.runtimeController = nil
	m.evidence = RuntimeEvidenceRepositories{}
	m.evidenceRecords = nil
	m.started = false
	m.driveMu.Lock()
	m.drivingRuns = nil
	m.retryLeaseTimers = nil
	m.driveMu.Unlock()
	m.startupRecoveryMu.Lock()
	m.runRuntimeCommandsReady = false
	m.startupRecoveryActions = nil
	m.startupRecoveryMu.Unlock()
	return nil
}

// handleRunStateChanged schedules SM-4 only after the Run owner has committed
// an active SM-1 state. It does not reconstruct authority or mutate any
// external owner state.
func (m *Biz) handleRunStateChanged(ev magicevent.Event, _ magicevent.Result) {
	data, ok := ev.Data().(runevents.RunStateChangedData)
	if ok && ev.Source() == runevents.CommandDestination && data.Validate() == nil && data.State == common.RunStateActive && !data.RequiresFreshActivation() {
		var scheduleErr error
		if requestID := strings.TrimSpace(data.ReconciliationRequestID); requestID != "" {
			scheduleErr = m.scheduleRuntimeDriveWithReconciliation(ev.Context(), data.WorkspaceID, data.RunID, "run_state_active_resume", requestID, 0)
		} else {
			scheduleErr = m.scheduleRuntimeDrive(ev.Context(), data.WorkspaceID, data.RunID, "run_state_active")
		}
		if err := scheduleErr; err != nil && !errors.Is(err, context.Canceled) {
			m.recordRuntimeDriveScheduleFailure(ev.Context(), data.WorkspaceID, data.RunID, "run_state_active", err)
		}
	}
}

func (m *Biz) handleRunContinuationRequested(ev magicevent.Event, _ magicevent.Result) {
	data, ok := ev.Data().(runevents.RunContinuationRequestedData)
	if !ok || ev.Source() != runevents.CommandDestination || data.Validate() != nil {
		return
	}
	data = data.Normalized()
	if err := m.scheduleRuntimeDriveWithReconciliation(ev.Context(), data.WorkspaceID, data.RunID, "run_continuation_requested", data.RequestID, 0); err != nil && !errors.Is(err, context.Canceled) {
		m.recordRuntimeDriveScheduleFailure(ev.Context(), data.WorkspaceID, data.RunID, "run_continuation_requested", err)
	}
}

// recordRuntimeDriveScheduleFailure makes a rejected BackgroundRoutine task
// visible in the persisted SM-4 recovery ledger. Each successful ledger CAS
// schedules one bounded recovery lease; restart inventory may still re-drive
// the same active Run after the local scheduling allowance is exhausted.
func (m *Biz) recordRuntimeDriveScheduleFailure(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string, scheduleErr error) {
	if m == nil || m.runtimeController == nil || workspaceID == "" || runID == "" {
		return
	}
	background := m.BackgroundRoutine()
	if background == nil {
		slog.Error("workorch Runtime drive scheduling rejected with no recovery routine", "source", source, "workspaceID", workspaceID, "runID", runID, "scheduleErr", scheduleErr)
		return
	}
	recoveryCtx := orchestratorBackgroundContext(ctx, workspaceID)
	// This path is called by a Run notification observer. Persisting through
	// the Run owner synchronously from that callback can deadlock its EventHub
	// lane, so recovery accounting is explicitly moved to a framework task.
	if err := background.AsyncFunction(func() {
		m.lifecycleMu.RLock()
		defer m.lifecycleMu.RUnlock()
		if m.runtimeController == nil {
			return
		}
		_, consumed, commitErr := m.runtimeConsumeRecoveryBudget(recoveryCtx, runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: workspaceID, RunID: runID}}, runtimecontract.RuntimeRecoveryBudgetDriveSchedule)
		if commitErr != nil {
			slog.Warn("workorch Runtime schedule recovery commit failed", "source", source, "workspaceID", workspaceID, "runID", runID, "scheduleErr", scheduleErr, "err", commitErr)
			return
		}
		if !consumed {
			slog.Error("workorch Runtime drive scheduling recovery exhausted; restart inventory will retry", "source", source, "workspaceID", workspaceID, "runID", runID, "scheduleErr", scheduleErr)
			return
		}
		if !m.beginRuntimeDrive(runtimeDriveKey(workspaceID, runID)) {
			return
		}
		if !m.scheduleRuntimeRecoveryLease(recoveryCtx, workspaceID, runID, runtimeRecoveryLeaseRequest{
			Reason: runtimecontract.RuntimeDriveLeaseReasonScheduleRetry, FailureSignature: "drive-schedule-rejected", Delay: time.Second, Source: source,
		}) {
			m.endRuntimeDrive(runtimeDriveKey(workspaceID, runID))
		}
	}); err != nil {
		slog.Error("workorch Runtime schedule recovery task was not accepted", "source", source, "workspaceID", workspaceID, "runID", runID, "scheduleErr", scheduleErr, "err", err)
	}
}

func (m *Biz) handleMagicCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeMagicCommand(ev)
	setOrchestratorCommandResult(result, response, err)
}

func (m *Biz) executeMagicCommand(ev magicevent.Event) (orchestratorevents.OrchestratorCommandResult, error) {
	if m == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	// Command handlers can race with framework teardown after their EventHub
	// subscription was already queued. Keep the owner dependencies live for the
	// full typed command; this does not grant SM-1 authority to choose an action.
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	return m.executeMagicCommandLocked(ev)
}

func (m *Biz) executeMagicCommandLocked(ev magicevent.Event) (orchestratorevents.OrchestratorCommandResult, error) {
	if m == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case orchestratorevents.TopicOrchestratorCommandPurgeRun:
		cmd, ok := ev.Data().(orchestratorevents.PurgeRunCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		var err error
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return orchestratorevents.OrchestratorCommandResult{}, err
		}
		runResult, err := m.purgeRunOwnerResources(ctx, cmd.WorkspaceID, cmd.RunID)
		if err != nil {
			return orchestratorevents.OrchestratorCommandResult{}, err
		}
		deleted, err := purgeRunEvidence(ctx, m.evidence, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{DeletedCount: runResult.DeletedCount + deleted}, err
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
		orchestratorevents.TopicOrchestratorCommandListPatchesByRun,
		orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun,
		orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
		orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
		return m.readEvidenceMagicCommand(ev)
	default:
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
}

func setOrchestratorCommandResult(result magicevent.Result, data orchestratorevents.OrchestratorCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func setOrchestratorAck(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(nil, nil)
}

func (m *Biz) scheduleRuntimeDrive(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string) error {
	return m.scheduleRuntimeDriveWithLeaseAndReconciliation(ctx, workspaceID, runID, source, 0, "", 0)
}

// scheduleRuntimeDriveWithReconciliation is used by the Run-owner
// continuation fact.  The request identity is carried only until the
// Orchestrator persists the next reconciliation generation; it is never a
// target state or a capability-owner decision.
func (m *Biz) scheduleRuntimeDriveWithReconciliation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source, reconciliationRequestID string, reconciliationGeneration uint64) error {
	return m.scheduleRuntimeDriveWithLeaseAndReconciliation(ctx, workspaceID, runID, source, 0, reconciliationRequestID, reconciliationGeneration)
}

// scheduleRuntimeDriveWithLease runs an already-claimed persisted lease
// without releasing the process-local gate in the handoff between timer and
// drive. A normal Start/Resume has generation zero and first acquires the gate.
func (m *Biz) scheduleRuntimeDriveWithLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string, leaseGeneration uint64) error {
	return m.scheduleRuntimeDriveWithLeaseAndReconciliation(ctx, workspaceID, runID, source, leaseGeneration, "", 0)
}

func (m *Biz) scheduleRuntimeDriveWithLeaseAndReconciliation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string, leaseGeneration uint64, reconciliationRequestID string, reconciliationGeneration uint64) error {
	if m == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	driveKey := runtimeDriveKey(workspaceID, runID)
	if leaseGeneration == 0 && !m.beginRuntimeDrive(driveKey) {
		return nil
	}
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: workspaceID, RunID: runID, Kind: orchestratorevents.RuntimeDriveObservationScheduled,
		RecoveryGeneration: leaseGeneration, Source: source,
	})
	drive := func(driveCtx context.Context) {
		var retry *runtimeRecoveryLeaseRequest
		freshActivationRequired := false
		// Keep the Runtime authority and its EventHub-backed dependencies
		// alive until this entire drive/projection/recovery branch returns.
		// A task that starts after Stop observes nil and exits without effects.
		m.lifecycleMu.RLock()
		if m.runtimeController == nil || m.EventHub() == nil {
			m.lifecycleMu.RUnlock()
			m.endRuntimeDrive(driveKey)
			return
		}
		defer m.lifecycleMu.RUnlock()
		m.observeRuntimeDrive(driveCtx, orchestratorevents.RuntimeDriveObservedData{
			WorkspaceID: workspaceID, RunID: runID, Kind: orchestratorevents.RuntimeDriveObservationRunning,
			RecoveryGeneration: leaseGeneration, LeaseState: runtimeDriveLeaseStateForGeneration(leaseGeneration), Source: source,
		})
		// Register after the lifecycle unlock defer so all owner-local lease
		// transitions complete while the Biz dependencies are still live.
		defer func() {
			if retry != nil && m.scheduleRuntimeRecoveryLease(driveCtx, workspaceID, runID, *retry) {
				// Retain the local singleflight key while the persisted lease is
				// waiting. The aggregate lease remains the cross-process authority.
				return
			}
			if !freshActivationRequired && leaseGeneration != 0 {
				if _, err := m.runtimeReleaseDriveLease(driveCtx, workspaceID, runID, leaseGeneration); err != nil && !errors.Is(err, context.Canceled) {
					slog.Warn("workorch Runtime drive lease release failed", "workspaceID", workspaceID, "runID", runID, "generation", leaseGeneration, "err", err)
				}
			}
			m.endRuntimeDrive(driveKey)
		}()
		if err := m.driveAndProjectRuntimeLockedWithCommand(driveCtx, orchestratorevents.DriveRuntimeCommand{
			WorkspaceID: workspaceID, RunID: runID, ReconciliationRequestID: reconciliationRequestID, ReconciliationGeneration: reconciliationGeneration,
		}); err != nil && !errors.Is(err, context.Canceled) {
			var fresh *runtimeFreshActivationRequiredError
			if errors.As(err, &fresh) {
				freshActivationRequired = true
				return
			}
			var persistedPending *runtimePersistedDrivePendingError
			if errors.As(err, &persistedPending) {
				slog.Debug("workorch Runtime persisted bounded retry state", "source", source, "workspaceID", workspaceID, "runID", runID, "revision", persistedPending.Revision)
				retry = &runtimeRecoveryLeaseRequest{Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: fmt.Sprintf("persisted-pending/%d", persistedPending.Revision), Delay: time.Second, Source: source}
				return
			}
			var projectionErr *runtimeProjectionError
			if errors.As(err, &projectionErr) {
				m.observeRuntimeDrive(driveCtx, orchestratorevents.RuntimeDriveObservedData{
					WorkspaceID: workspaceID, RunID: runID, Kind: orchestratorevents.RuntimeDriveObservationDeliveryPending,
					RecoveryGeneration: leaseGeneration, FailureCategory: "terminal_delivery_pending", LeaseState: runtimeDriveLeaseStateForGeneration(leaseGeneration), Source: source,
				})
				slog.Warn("workorch Runtime projection failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", err)
				_, consumed, budgetErr := m.runtimeConsumeRecoveryBudget(driveCtx, runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: workspaceID, RunID: runID}}, runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
				if budgetErr != nil {
					slog.Warn("workorch Runtime terminal delivery budget commit failed", "workspaceID", workspaceID, "runID", runID, "err", budgetErr)
					return
				}
				if consumed {
					retry = &runtimeRecoveryLeaseRequest{Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery", Delay: time.Second, Source: source}
				}
				return
			}
			slog.Warn("workorch Runtime drive failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", err)
			// RVC-R13: CAS conflict / owner transient / delivery failures must not
			// terminal a business Run as runtime contract defect.
			failure := classifyRuntimeDriveFailure(err)
			if revision, observed := runtimeDriveFailureObservedRevision(err); observed {
				kind := orchestratorevents.RuntimeDriveObservationCASReload
				if failure.Code != "cas_conflict" {
					kind = orchestratorevents.RuntimeDriveObservationSemanticRejection
				}
				m.observeRuntimeDrive(driveCtx, orchestratorevents.RuntimeDriveObservedData{
					WorkspaceID: workspaceID, RunID: runID, Kind: kind, AggregateRevision: revision,
					RecoveryGeneration: leaseGeneration, FailureCategory: failure.Code, LeaseState: runtimeDriveLeaseStateForGeneration(leaseGeneration), Source: source,
				})
			}
			if !runtime.FailureClassIsBusinessTerminal(failure.Class) {
				slog.Warn("workorch Runtime drive non-terminal failure", "source", source, "workspaceID", workspaceID, "runID", runID, "class", failure.Class, "err", err)
				if budgetKind, ok := runtimeRecoveryBudgetKind(failure.Class); ok {
					_, consumed, budgetErr := m.runtimeConsumeRecoveryBudget(driveCtx, runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: workspaceID, RunID: runID}}, budgetKind)
					if budgetErr != nil {
						slog.Warn("workorch Runtime recovery budget commit failed", "workspaceID", workspaceID, "runID", runID, "kind", budgetKind, "err", budgetErr)
						return
					}
					if consumed {
						retry = runtimeLeaseRequestForFailure(failure, err, source, time.Second)
					}
					if !consumed && failure.Class == runtimecontract.FailureClassOwnerTransientUnavailable {
						if _, recordErr := m.submitRuntimeInternalEvent(driveCtx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventDriveFailureObserved, WorkspaceID: workspaceID, RunID: runID, FailureFact: &failure}); recordErr != nil {
							slog.Warn("workorch Runtime exhausted operation recovery fact commit failed", "workspaceID", workspaceID, "runID", runID, "err", recordErr)
						} else {
							retry = &runtimeRecoveryLeaseRequest{Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "owner-transient-terminal", Delay: time.Second, Source: source}
						}
					}
					if !consumed && failure.Class == runtimecontract.FailureClassCASConflict {
						// The recovery ledger is bounded for observability, not a
						// license to strand an active aggregate. A CAS conflict has
						// no business-terminal meaning: reload/re-drive remains an
						// Orchestrator-local SM-4 responsibility until the physical
						// persistence race clears or a process restart inventories it.
						// Back off after the persisted budget is exhausted so a
						// pathological writer cannot spin the owner lane.
						retry = runtimeLeaseRequestForFailure(failure, err, source, 5*time.Second)
						slog.Error("workorch Runtime CAS recovery ledger exhausted; keeping owner-local re-drive pending", "source", source, "workspaceID", workspaceID, "runID", runID)
					}
				}
				return
			}
			recorded, recordErr := m.submitRuntimeInternalEvent(driveCtx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventDriveFailureObserved, WorkspaceID: workspaceID, RunID: runID, FailureFact: &failure})
			if recordErr != nil {
				slog.Warn("workorch Runtime drive failure fact commit failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", recordErr)
				return
			}
			aggregate := recorded.Aggregate
			if aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || aggregate.Decision.Terminal == nil {
				slog.Warn("workorch Runtime drive failure did not reduce to terminal", "source", source, "workspaceID", workspaceID, "runID", runID)
				return
			}
			if projectErr := m.deliverRuntimeTerminal(driveCtx, aggregate); projectErr != nil {
				m.observeRuntimeDrive(driveCtx, orchestratorevents.RuntimeDriveObservedData{
					WorkspaceID: workspaceID, RunID: runID, Kind: orchestratorevents.RuntimeDriveObservationDeliveryPending,
					AggregateRevision: aggregate.StateRevision, RecoveryGeneration: leaseGeneration, FailureCategory: "terminal_delivery_pending", LeaseState: runtimeDriveLeaseStateForGeneration(leaseGeneration), Source: source,
				})
				slog.Warn("workorch Runtime drive failure delivery failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", projectErr)
				_, consumed, budgetErr := m.runtimeConsumeRecoveryBudget(driveCtx, runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: workspaceID, RunID: runID}}, runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
				if budgetErr != nil {
					slog.Warn("workorch Runtime terminal delivery budget commit failed", "workspaceID", workspaceID, "runID", runID, "err", budgetErr)
					return
				}
				if consumed {
					retry = &runtimeRecoveryLeaseRequest{Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: "terminal-delivery", Delay: time.Second, Source: source}
				}
			}
		}
	}
	bgCtx := orchestratorBackgroundContext(ctx, workspaceID)
	// A timer already runs inside BackgroundRoutine. Execute the claimed lease
	// there so the persisted running lease and local gate have no handoff gap.
	if m.BackgroundRoutine() == nil || leaseGeneration != 0 {
		drive(bgCtx)
		return nil
	}
	if err := m.BackgroundRoutine().AsyncFunction(func() { drive(bgCtx) }); err != nil {
		m.endRuntimeDrive(driveKey)
		return err
	}
	return nil
}

type runtimeRecoveryLeaseRequest struct {
	Reason           runtimecontract.RuntimeDriveLeaseReason
	FailureSignature string
	Delay            time.Duration
	Source           string
}

func runtimeDriveKey(workspaceID common.WorkspaceID, runID common.RunID) string {
	return string(workspaceID) + "\x00" + string(runID)
}

func runtimeLeaseRequestForFailure(failure runtimecontract.FailureFact, err error, source string, delay time.Duration) *runtimeRecoveryLeaseRequest {
	reason := runtimecontract.RuntimeDriveLeaseReasonOperationRetry
	if failure.Class == runtimecontract.FailureClassCASConflict {
		reason = runtimecontract.RuntimeDriveLeaseReasonCASReload
		var cas *runtime.RuntimeCASConflictError
		if errors.As(err, &cas) {
			return &runtimeRecoveryLeaseRequest{Reason: reason, FailureSignature: fmt.Sprintf("cas/%d/%d", cas.ExpectedRevision, cas.ObservedRevision), Delay: delay, Source: source}
		}
	}
	return &runtimeRecoveryLeaseRequest{Reason: reason, FailureSignature: string(failure.Class) + "/" + failure.Signature, Delay: delay, Source: source}
}

// scheduleRuntimeRecoveryLease persists retry identity before the timer is
// started. Returning true means the caller must retain its singleflight gate.
func (m *Biz) scheduleRuntimeRecoveryLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, request runtimeRecoveryLeaseRequest) bool {
	if m == nil || m.runtimeController == nil || workspaceID == "" || runID == "" || request.Reason == "" || request.FailureSignature == "" {
		return false
	}
	if request.Delay <= 0 {
		request.Delay = time.Second
	}
	lease, _, err := m.runtimeScheduleDriveLease(ctx, workspaceID, runID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: workspaceID, RunID: runID, Reason: request.Reason, FailureSignature: request.FailureSignature,
		DueAt: time.Now().UTC().Add(request.Delay).Format(time.RFC3339Nano),
	})
	if err != nil {
		slog.Warn("workorch Runtime recovery lease commit failed", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "reason", request.Reason, "signature", request.FailureSignature, "err", err)
		return false
	}
	if lease.State != runtimecontract.RuntimeDriveLeaseScheduled || !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, request.Source) {
		slog.Warn("workorch Runtime recovery lease timer was not accepted", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "generation", lease.RecoveryGeneration)
		return false
	}
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: request.Source,
	})
	slog.Info("workorch Runtime recovery lease scheduled", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "revision", lease.AggregateRevision, "generation", lease.RecoveryGeneration, "reason", lease.Reason, "signature", lease.FailureSignature, "state", lease.State, "dueAt", lease.DueAt)
	return true
}

func (m *Biz) recoverRuntimeDriveLease(ctx context.Context, aggregate runtime.RuntimeAggregate, source string) (runtimecontract.RuntimeDriveLease, error) {
	if aggregate.DriveLease == nil {
		return runtimecontract.RuntimeDriveLease{}, common.ErrNotFound
	}
	lease := aggregate.DriveLease.Clone()
	if lease.State == runtimecontract.RuntimeDriveLeaseRunning {
		var err error
		lease, _, err = m.runtimeRecoverDriveLease(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return runtimecontract.RuntimeDriveLease{}, err
		}
	}
	if lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return runtimecontract.RuntimeDriveLease{}, common.ErrInvalidArgument
	}
	slog.Info("workorch Runtime recovery lease recovered", "source", source, "workspaceID", lease.WorkspaceID, "runID", lease.RunID, "revision", lease.AggregateRevision, "generation", lease.RecoveryGeneration, "reason", lease.Reason, "signature", lease.FailureSignature, "state", lease.State)
	return lease, nil
}

func (m *Biz) enqueueRuntimeDriveLeaseTimer(ctx context.Context, lease runtimecontract.RuntimeDriveLease, source string) bool {
	if m == nil || m.BackgroundRoutine() == nil || lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return false
	}
	key := runtimeDriveKey(lease.WorkspaceID, lease.RunID)
	if !m.beginRuntimeLeaseTimer(key, lease.RecoveryGeneration) {
		return true
	}
	retryCtx := orchestratorBackgroundContext(ctx, lease.WorkspaceID)
	if err := m.BackgroundRoutine().AsyncFunction(func() {
		defer m.endRuntimeLeaseTimer(key, lease.RecoveryGeneration)
		dueAt, parseErr := time.Parse(time.RFC3339Nano, lease.DueAt)
		if parseErr != nil {
			m.endRuntimeDrive(key)
			return
		}
		if delay := time.Until(dueAt); delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-retryCtx.Done():
				m.endRuntimeDrive(key)
				return
			case <-timer.C:
			}
		}
		m.observeRuntimeDrive(retryCtx, orchestratorevents.RuntimeDriveObservedData{
			WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerFired,
			AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: source,
		})
		claimedLease, claimed, claimErr := m.runtimeClaimDriveLease(retryCtx, lease, time.Now().UTC().Format(time.RFC3339Nano))
		if claimErr != nil {
			slog.Warn("workorch Runtime recovery lease claim failed", "source", source, "workspaceID", lease.WorkspaceID, "runID", lease.RunID, "generation", lease.RecoveryGeneration, "err", claimErr)
			m.endRuntimeDrive(key)
			return
		}
		if !claimed {
			if claimedLease.WorkspaceID != "" && claimedLease.State == runtimecontract.RuntimeDriveLeaseScheduled && m.enqueueRuntimeDriveLeaseTimer(retryCtx, claimedLease, source) {
				return
			}
			m.endRuntimeDrive(key)
			return
		}
		if err := m.scheduleRuntimeDriveWithLease(retryCtx, claimedLease.WorkspaceID, claimedLease.RunID, "recovery_lease:"+source, claimedLease.RecoveryGeneration); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch Runtime claimed recovery drive scheduling failed", "workspaceID", lease.WorkspaceID, "runID", lease.RunID, "generation", claimedLease.RecoveryGeneration, "err", err)
			m.endRuntimeDrive(key)
		}
	}); err != nil {
		m.endRuntimeLeaseTimer(key, lease.RecoveryGeneration)
		return false
	}
	return true
}

func runtimeRecoveryBudgetKind(class runtimecontract.FailureClass) (runtimecontract.RuntimeRecoveryBudgetKind, bool) {
	switch runtime.FailureClassDisposition(class) {
	case runtime.FailureDispositionProviderRetry:
		return runtimecontract.RuntimeRecoveryBudgetProviderTransport, true
	case runtime.FailureDispositionOperationRecovery:
		return runtimecontract.RuntimeRecoveryBudgetOperationRecovery, true
	case runtime.FailureDispositionCASReload:
		return runtimecontract.RuntimeRecoveryBudgetCASReload, true
	case runtime.FailureDispositionDeliveryRetry:
		return runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, true
	default:
		return "", false
	}
}

func (m *Biz) beginRuntimeDrive(key string) bool {
	if m == nil || key == "" {
		return false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.drivingRuns == nil {
		m.drivingRuns = map[string]struct{}{}
	}
	if _, ok := m.drivingRuns[key]; ok {
		return false
	}
	m.drivingRuns[key] = struct{}{}
	return true
}

func (m *Biz) endRuntimeDrive(key string) {
	if m == nil || key == "" {
		return
	}
	m.driveMu.Lock()
	delete(m.drivingRuns, key)
	m.driveMu.Unlock()
}

func (m *Biz) beginRuntimeLeaseTimer(key string, generation uint64) bool {
	if m == nil || key == "" || generation == 0 {
		return false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.retryLeaseTimers == nil {
		m.retryLeaseTimers = map[string]uint64{}
	}
	if current, ok := m.retryLeaseTimers[key]; ok && current == generation {
		return false
	}
	m.retryLeaseTimers[key] = generation
	return true
}

func (m *Biz) endRuntimeLeaseTimer(key string, generation uint64) {
	if m == nil || key == "" || generation == 0 {
		return
	}
	m.driveMu.Lock()
	if m.retryLeaseTimers != nil && m.retryLeaseTimers[key] == generation {
		delete(m.retryLeaseTimers, key)
	}
	m.driveMu.Unlock()
}

func orchestratorBackgroundContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	bgCtx := context.Background()
	if workspaceID != "" {
		bgCtx, _ = common.BindRuntimeWorkspaceContext(bgCtx, workspaceID)
	} else if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		bgCtx, _ = common.BindRuntimeWorkspaceContext(bgCtx, workspaceID)
	}

	return bgCtx
}

func (m *Biz) readEvidenceMagicCommand(ev magicevent.Event) (orchestratorevents.OrchestratorCommandResult, error) {
	if m == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := listRunEvidenceCommand(ev.ID(), ev.Data())
	if !ok || cmd.RunID == "" {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	if cmd.WorkspaceID != "" {
		var err error
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return orchestratorevents.OrchestratorCommandResult{}, err
		}
	}
	evidence := m.evidence
	switch ev.ID() {
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun:
		if evidence.ToolCalls == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ToolCalls.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ToolCalls: runtimeevidencemodel.CloneToolCalls(items)}, err
	case orchestratorevents.TopicOrchestratorCommandListPatchesByRun:
		if evidence.Patches == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.Patches.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{Patches: runtimeevidencemodel.ClonePatches(items)}, err
	case orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun:
		if evidence.ValidationResults == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ValidationResults.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ValidationResults: runtimeevidencemodel.CloneValidationResults(items)}, err
	case orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun:
		if evidence.CoverageWorkItems == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.CoverageWorkItems.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{CoverageWorkItems: runtimecontract.CloneCoverageWorkItems(items)}, err
	case orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
		if evidence.ArtifactContinuations == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ArtifactContinuations.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ArtifactContinuations: orchestratorevents.ArtifactContinuationsFromModels(items)}, err
	default:
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
}

func listRunEvidenceCommand(topic string, data any) (orchestratorevents.ListRunEvidenceCommand, bool) {
	switch topic {
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun:
		cmd, ok := data.(orchestratorevents.ListToolCallsByRunCommand)
		return orchestratorevents.ListRunEvidenceCommand(cmd), ok
	case orchestratorevents.TopicOrchestratorCommandListPatchesByRun:
		cmd, ok := data.(orchestratorevents.ListPatchesByRunCommand)
		return orchestratorevents.ListRunEvidenceCommand(cmd), ok
	case orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun:
		cmd, ok := data.(orchestratorevents.ListValidationResultsByRunCommand)
		return orchestratorevents.ListRunEvidenceCommand(cmd), ok
	case orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun:
		cmd, ok := data.(orchestratorevents.ListCoverageWorkItemsByRunCommand)
		return orchestratorevents.ListRunEvidenceCommand(cmd), ok
	case orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
		cmd, ok := data.(orchestratorevents.ListArtifactContinuationsByRunCommand)
		return orchestratorevents.ListRunEvidenceCommand(cmd), ok
	default:
		return orchestratorevents.ListRunEvidenceCommand{}, false
	}
}

func runtimeRunIsTerminal(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func classifyRuntimeDriveFailure(err error) runtimecontract.FailureFact {
	if err == nil {
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeInvariantDefect, Code: "runtime_drive_failed", Signature: "runtime-drive-failed", Message: "unknown drive failure"}
	}
	msg := err.Error()
	var unsupportedCapability *runtimecontract.RuntimeCapabilityUnsupportedError
	var capabilityCatalogUnavailable *runtimecontract.RuntimeCapabilityCatalogUnavailableError
	var outcomeUnknown *runtime.OperationOutcomeUnknownError
	var casConflict *runtime.RuntimeCASConflictError
	var semanticRejection *runtime.RuntimeSemanticRejectionError
	var timeout interface{ Timeout() bool }
	var temporary interface{ Temporary() bool }
	switch {
	case errors.As(err, &capabilityCatalogUnavailable):
		return runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassCapabilityUnavailable, Code: "mcp_capability_catalog_unavailable",
			Signature: "mcp-capability-catalog-unavailable", Message: msg,
		}
	case errors.As(err, &unsupportedCapability):
		return runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_capability_unsupported",
			Signature: "runtime-capability-unsupported", Message: msg,
		}
	case errors.As(err, &casConflict):
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassCASConflict, Code: "cas_conflict", Signature: "cas-conflict", Message: msg}
	case errors.As(err, &semanticRejection):
		// A Run-owner semantic rejection is a typed persistence fact, not a
		// physical CAS conflict and not a textual error classifier input.  It
		// returns to the owner-local reducer below as a terminal contract fact
		// while preserving the producer's complete safe diagnostic for audit.
		feedback := semanticRejection.Feedback.CloneOriginal()
		failure := runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment,
			Code:  "runtime_semantic_rejection", Signature: "runtime-semantic-rejection", Message: msg,
		}
		if original := feedback.OriginalDiagnostic; original != nil {
			failure.Code = original.OriginalCode
			failure.Signature = original.Signature
			failure.Message = original.OriginalMessage
			failure.TargetRefs = append([]string(nil), original.TargetRefs...)
			failure.OriginalDiagnostic = original.CloneOriginal()
		}
		return failure
	case errors.Is(err, common.ErrConflict):
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_semantic_conflict", Signature: "runtime-semantic-conflict", Message: msg}
	case errors.As(err, &outcomeUnknown):
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassOperationOutcomeUnknown, Code: "operation_outcome_unknown", Signature: "operation-outcome-unknown", Message: msg}
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout(), errors.As(err, &temporary) && temporary.Temporary():
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassOwnerTransientUnavailable, Code: "owner_transient", Signature: "owner-transient", Message: msg}
	default:
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "runtime-drive-failed", Message: msg}
	}
}

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
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Evidence          repository.OrchestratorEvidenceOwnerRepositories
	CommandHub        magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	runtimeController  *runtime.RuntimeController
	evidence           RuntimeEvidenceRepositories
	evidenceRecords    repository.EvidenceRecordRepository
	workspaceDeletions repository.OrchestratorWorkspaceDeletionRepository
	workspacePurger    repository.OrchestratorWorkspaceEvidencePurger
	rawTopics          []string
	// startupRecoveryActions are owner-local lifecycle handoff data only. They
	// are rebuilt from the persisted Run inventory on every process start and
	// are released solely by the Run owner's commands-ready fact; they are not
	// a second persisted Runtime state machine.
	startupRecoveryMu       sync.Mutex
	runRuntimeCommandsReady bool
	startupRecoveryActions  []runtimeStartupRecoveryAction
	// lifecycleStopMu serializes framework Start/Stop without becoming a Runtime
	// state-machine lock. lifecycleMu protects Runtime owner dependencies while
	// an ingress handler or background drive is using them. Stop takes the write
	// lock before clearing pointers; every drive holds the read lock through
	// authority and projection work.
	lifecycleStopMu sync.Mutex
	lifecycleMu     sync.RWMutex
	// runtimeMutationFence linearizes the durable Workspace quiescing gate and
	// final quiescence token with every owner-private Runtime aggregate CAS.
	runtimeMutationFence sync.RWMutex
	driveMu              sync.Mutex
	// drivingRuns records the lifecycle epoch which owns each process-local
	// gate. Epoch fencing prevents a retiring timer from an earlier Start/Stop
	// lifecycle from deleting a newly started gate for the same run.
	drivingRuns map[string]uint64
	// retiredWorkspaces is a process-local gate derived exclusively from the
	// Run owner's durable retirement head. It is rebuilt before startup
	// recovery and never becomes a second persisted deletion authority.
	retiredWorkspaces   map[common.WorkspaceID]runevents.RunWorkspaceRetirementFence
	quiescingWorkspaces map[common.WorkspaceID]deletioncontract.WorkspaceRuntimeQuiescenceIntent
	// persistedLeaseMounts is a process-local generation latch used only when
	// an SM-4 lifecycle delivery lease is persisted while another worker owns
	// the run-scoped drive gate. The persisted RuntimeDriveLease remains the
	// sole retry authority; the current gate owner consumes this hint on exit
	// and mounts that exact lease onto BackgroundRoutine without a second drive.
	persistedLeaseMounts map[string]runtimecontract.RuntimeDriveLease
	// retryLeaseTimers is an in-process dedupe for an already persisted SM-4
	// lease. It is not retry authority and is discarded during process restart.
	retryLeaseTimers         map[string]runtimeLeaseTimerRegistration
	runtimeLeaseTimerCtx     context.Context
	runtimeLeaseTimerCancel  context.CancelFunc
	lifecycleCtx             context.Context
	lifecycleCancel          context.CancelFunc
	runtimeLeaseTimerEpoch   uint64
	runtimeLeaseTimerClosing bool
	started                  bool
}

type runtimeLeaseTimerRegistration struct {
	Generation uint64
	Epoch      uint64
}

type runtimeStartupRecoveryActionKind string

const (
	runtimeStartupRecoveryActionDrive        runtimeStartupRecoveryActionKind = "drive"
	runtimeStartupRecoveryActionRecoverLease runtimeStartupRecoveryActionKind = "recover_lease"
)

type runtimeStartupRecoveryAction struct {
	Kind   runtimeStartupRecoveryActionKind
	Item   runevents.RuntimeRecoveryInventoryItem
	Source string
}

func New() *Biz { return &Biz{} }

func (m *Biz) ID() string { return ModuleID }

func (m *Biz) Configure(deps Dependencies) error {
	if deps.Evidence.Validate() != nil || deps.CommandHub == nil || deps.BackgroundRoutine == nil {
		return common.ErrInvalidArgument
	}
	m.Base = basebiz.New(ModuleID, deps.CommandHub, deps.BackgroundRoutine)
	m.evidence = RuntimeEvidenceRepositories{
		ToolCalls:             deps.Evidence.ToolCalls(),
		Patches:               deps.Evidence.Patches(),
		ValidationResults:     deps.Evidence.ValidationResults(),
		EvidenceRecords:       deps.Evidence.EvidenceRecords(),
		CoverageWorkItems:     deps.Evidence.CoverageWorkItems(),
		ArtifactContinuations: deps.Evidence.ArtifactContinuations(),
	}
	controller, err := runtime.NewRuntimeController(runtimeStateRepository{hub: m.EventHub()})
	if err != nil {
		return err
	}
	m.runtimeController = controller
	m.evidenceRecords = deps.Evidence.EvidenceRecords()
	m.workspaceDeletions = deps.Evidence.WorkspaceDeletions()
	m.workspacePurger = deps.Evidence.WorkspacePurger()
	return nil
}

func (m *Biz) Start() error {
	return m.StartContext(context.Background())
}

// StartContext binds all owner-local background drive and timer work to the
// framework lifecycle. Setup has already supplied the shared scheduler; a
// missing scheduler or a cancelled framework context is a startup failure.
func (m *Biz) StartContext(ctx context.Context) error {
	if m == nil || ctx == nil || m.runtimeController == nil || m.EventHub() == nil || m.BackgroundRoutine() == nil {
		return common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.lockLifecycleTransition(ctx); err != nil {
		return err
	}
	defer m.lifecycleStopMu.Unlock()
	if m.started {
		return nil
	}
	if err := m.openRuntimeLifecycle(ctx); err != nil {
		return err
	}
	startupSucceeded := false
	defer func() {
		if !startupSucceeded {
			m.rollbackRuntimeStart()
		}
	}()
	lifecycleCtx := m.runtimeLifecycleContext()
	m.driveMu.Lock()
	m.retiredWorkspaces = make(map[common.WorkspaceID]runevents.RunWorkspaceRetirementFence)
	m.quiescingWorkspaces = make(map[common.WorkspaceID]deletioncontract.WorkspaceRuntimeQuiescenceIntent)
	m.driveMu.Unlock()
	// Durable Orchestrator gates are restored before either Run retirement
	// inventory or ordinary SM-4 recovery is classified. A fresh process must
	// never briefly reopen a workspace that was already quiesced.
	if err := m.recoverWorkspaceRuntimeQuiescenceGates(lifecycleCtx); err != nil {
		return fmt.Errorf("Runtime startup Workspace quiescence inventory: %w", err)
	}
	if err := m.recoverWorkspaceRetirementGates(lifecycleCtx); err != nil {
		return fmt.Errorf("Runtime startup retirement inventory: %w", err)
	}
	m.subscribeRuntimeIngress()
	// GMM-R1 / SM-4: recovery inventory must be acquired synchronously. A missing
	// Run owner subscriber is a startup failure, not a warning that loses recovery.
	if err := m.recoverRuntimeStartup(lifecycleCtx); err != nil {
		return err
	}
	if err := lifecycleCtx.Err(); err != nil {
		return err
	}
	m.started = true
	startupSucceeded = true
	return nil
}

func (m *Biz) subscribeRuntimeIngress() {
	if m == nil || len(m.rawTopics) != 0 {
		return
	}
	m.rawTopics = []string{
		// RUNTIME-SMO: Run lifecycle topics are owned by blocks/run. The
		orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence,
		orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence,
		orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion,
		orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence,
		orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
		orchestratorevents.TopicOrchestratorCommandListPatchesByRun,
		orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun,
		orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
		orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun,
	}
	for _, topic := range m.rawTopics {
		switch topic {
		case orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence,
			orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence:
			m.SubscribeFunc(topic, m.handleWorkspaceRuntimeQuiescence)
		case orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion,
			orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence:
			m.SubscribeFunc(topic, m.handleWorkspaceDeletion)
		default:
			m.SubscribeFunc(topic, m.handleMagicCommand)
		}
	}
	m.rawTopics = append(m.rawTopics,
		orchestratorevents.TopicOrchestratorDriveRuntime,
		orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
		orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt,
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval,
		orchestratorevents.TopicOrchestratorRequestRuntimeTermination,
		orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence,
		orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence,
	)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorDriveRuntime, m.handleRuntimeDrive)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, m.handleRuntimeOperationReceipt)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, m.handleRuntimeOperationReceipt)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, m.handleRuntimeGovernanceTicketResolution)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval, m.handleRuntimeGovernanceApprovalResolution)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, m.handleRuntimeTerminationRequest)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence, m.handleDecisionEvidenceCommand)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence, m.handleApprovalEvidenceCommand)
	m.SubscribeMatchedFunc(ModuleID+"/run-drive", runevents.NotificationDestination, runevents.TopicRunStateChanged, m.handleRunStateChanged)
	m.SubscribeMatchedFunc(ModuleID+"/run-drive", runevents.NotificationDestination, runevents.TopicRunContinuationRequested, m.handleRunContinuationRequested)
	m.SubscribeMatchedFunc(ModuleID+"/run-drive", runevents.NotificationDestination, runevents.TopicRunRuntimeCommandsReady, m.handleRunRuntimeCommandsReady)
}

func (m *Biz) handleRuntimeTerminationRequest(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(orchestratorevents.RequestRuntimeTerminationCommand)
	// A termination request is only the persisted supersede fact emitted by
	// the Run owner.  No application/UI/governance caller may turn a command
	// into an SM-4 transition directly.
	if !ok || ev.Source() != "blocks/run" || m == nil || cmd.Validate() != nil || cmd.RequestedBy != "blocks/run" {
		setOrchestratorAck(result, common.ErrInvalidArgument)
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	if m.runtimeController == nil {
		setOrchestratorAck(result, common.ErrInvalidArgument)
		return
	}
	terminated, err := m.submitRuntimeInternalEvent(ev.Context(), runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventTerminationRequested, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{TerminationKind: cmd.Kind, RequestedBy: cmd.RequestedBy, Reason: cmd.Reason},
	})
	if err != nil {
		setOrchestratorAck(result, err)
		return
	}
	aggregate := terminated.Aggregate
	request, err := terminalDeliveryRequest(aggregate)
	if err != nil {
		setOrchestratorAck(result, err)
		return
	}
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
		m.runtimeMutationFence.RLock()
		_, quiescing := m.quiescingWorkspaces[item.WorkspaceID]
		m.runtimeMutationFence.RUnlock()
		if quiescing {
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
			return fmt.Errorf("Runtime startup recovery load workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, loadErr)
		}
		if fence, observed := m.workspaceRetirementGate(item.WorkspaceID); observed {
			if runWorkspaceRetirementFenceIsRetired(fence) {
				continue
			}
			if !runWorkspaceRetirementFenceIsDraining(fence) {
				return fmt.Errorf("Runtime startup recovery workspace=%s has unsupported Run retirement phase", item.WorkspaceID)
			}
			if _, terminalErr := terminalDeliveryRequest(aggregate); terminalErr != nil {
				return fmt.Errorf("Runtime startup recovery workspace=%s run=%s draining aggregate is not terminal: %w", item.WorkspaceID, item.RunID, terminalErr)
			}
			if aggregate.DriveLease != nil {
				if aggregate.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry {
					return fmt.Errorf("%w: Runtime startup recovery workspace=%s run=%s draining lease is not terminal delivery", common.ErrConflict, item.WorkspaceID, item.RunID)
				}
				actions = append(actions, runtimeStartupRecoveryAction{Kind: runtimeStartupRecoveryActionRecoverLease, Item: item, Source: "startup_recovery:workspace_retirement_drain"})
				continue
			}
			if !runtimeTerminalDeliveryFullyAcknowledged(aggregate) {
				actions = append(actions, runtimeStartupRecoveryAction{Kind: runtimeStartupRecoveryActionDrive, Item: item, Source: "startup_recovery:workspace_retirement_terminal_delivery"})
			}
			continue
		}

		// A persisted SM-4 lease survives process restart. It is resumed before
		// any normal classification so startup cannot create a parallel drive.
		if aggregate.DriveLease != nil {
			actions = append(actions, runtimeStartupRecoveryAction{Kind: runtimeStartupRecoveryActionRecoverLease, Item: item, Source: "startup_recovery"})
			continue
		}

		// Recovery classification is exclusively an Orchestrator SM-4 concern
		// after typed aggregate load. The Run inventory remains opaque.
		record := runtimeStateRecordFromAggregate(aggregate)
		lifecycleHandoff, governed, handoffErr := runtimeGovernanceLifecycleHandoffFromState(&record)
		if handoffErr != nil {
			return fmt.Errorf("Runtime startup recovery lifecycle identity workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, handoffErr)
		}
		blockedOnPersistedContinuation := governed &&
			((lifecycleHandoff.Kind == runtimeGovernanceLifecycleContinuationDecision && item.State == common.RunStateBlockedOnDecision) ||
				(lifecycleHandoff.Kind == runtimeGovernanceLifecycleContinuationApproval && item.State == common.RunStateBlockedOnApproval))
		source := ""
		switch {
		case runtimeTerminalDeliveryReady(aggregate):
			source = "startup_recovery:terminal_delivery"
		case blockedOnPersistedContinuation:
			// Resolution is durable but Run is still blocked: the crash cut lies
			// between the SM-4 CAS and the exact SM-1 continuation fact. Re-drive
			// the persisted handoff before any execution transition.
			source = "startup_recovery:governance_continuation_delivery"
		case aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance:
			// A crash can occur after the bound governance request and opaque
			// lifecycle fence were committed but before Run owner accepted the
			// immutable wait fact. Re-drive only that persisted handoff while the
			// Run is still lifecycle-eligible; an already-blocked Run remains a
			// genuine wait and is not made execution-eligible here.
			if item.State == common.RunStatePending || item.State == common.RunStateActive {
				source = "startup_recovery:governance_lifecycle_delivery"
			} else {
				continue
			}
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

const (
	runtimeRecoveryInventoryPageSize = 100
	runtimeRecoveryInventoryMaxPages = 1000
	runtimeRecoveryInventoryMaxItems = runtimeRecoveryInventoryPageSize * runtimeRecoveryInventoryMaxPages
)

func queryRuntimeRecoveryInventory(ctx context.Context, hub magicevent.Hub) ([]runevents.RuntimeRecoveryInventoryItem, error) {
	return queryRuntimeRecoveryInventoryBounded(
		ctx,
		hub,
		runtimeRecoveryInventoryPageSize,
		runtimeRecoveryInventoryMaxPages,
		runtimeRecoveryInventoryMaxItems,
	)
}

func queryRuntimeRecoveryInventoryBounded(ctx context.Context, hub magicevent.Hub, pageSize, maxPages, maxItems int) ([]runevents.RuntimeRecoveryInventoryItem, error) {
	if hub == nil || pageSize < 1 || maxPages < 1 || maxItems < 1 {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]runevents.RuntimeRecoveryInventoryItem, 0)
	cursor := ""
	visited := map[string]struct{}{cursor: {}}
	for pageNumber := 1; pageNumber <= maxPages; pageNumber++ {
		result := hub.Send(magicevent.NewEventWithContext(
			runevents.TopicRunCommandRuntimeRecoveryInventory, ModuleID, runevents.CommandDestination,
			magicevent.NewHeader(), ctx, runevents.QueryRuntimeRecoveryInventoryCommand{Cursor: cursor, Limit: pageSize},
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
		if len(page.Items) > maxItems-len(ret) {
			return nil, fmt.Errorf("%w: runtime recovery inventory exceeds %d items", common.ErrConflict, maxItems)
		}
		ret = append(ret, page.Items...)
		nextCursor := strings.TrimSpace(page.NextCursor)
		if nextCursor == "" {
			return ret, nil
		}
		if _, duplicate := visited[nextCursor]; duplicate {
			return nil, fmt.Errorf("%w: runtime recovery inventory cursor cycle at %q", common.ErrConflict, nextCursor)
		}
		if pageNumber == maxPages {
			return nil, fmt.Errorf("%w: runtime recovery inventory exceeds %d pages", common.ErrConflict, maxPages)
		}
		visited[nextCursor] = struct{}{}
		cursor = nextCursor
	}
	return nil, fmt.Errorf("%w: runtime recovery inventory exceeds %d pages", common.ErrConflict, maxPages)
}

func (m *Biz) openRuntimeLifecycle(ctx context.Context) error {
	if m == nil || ctx == nil {
		return common.ErrInvalidArgument
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.lifecycleCtx != nil || m.lifecycleCancel != nil {
		return fmt.Errorf("%w: orchestrator lifecycle is already open", common.ErrConflict)
	}
	if m.runtimeLeaseTimerEpoch == 0 {
		m.runtimeLeaseTimerEpoch = 1
	}
	m.runtimeLeaseTimerClosing = false
	m.lifecycleCtx, m.lifecycleCancel = context.WithCancel(ctx)
	m.runtimeLeaseTimerCtx, m.runtimeLeaseTimerCancel = context.WithCancel(m.lifecycleCtx)
	return nil
}

func (m *Biz) runtimeLifecycleContext() context.Context {
	if m == nil {
		return context.Background()
	}
	m.driveMu.Lock()
	ctx := m.lifecycleCtx
	m.driveMu.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (m *Biz) rollbackRuntimeStart() {
	if m == nil {
		return
	}
	m.unsubscribeRuntimeIngress()
	m.driveMu.Lock()
	if !m.runtimeLeaseTimerClosing {
		m.runtimeLeaseTimerClosing = true
		m.runtimeLeaseTimerEpoch++
	}
	if m.lifecycleCancel != nil {
		m.lifecycleCancel()
	}
	if m.runtimeLeaseTimerCancel != nil {
		m.runtimeLeaseTimerCancel()
	}
	m.driveMu.Unlock()
	m.startupRecoveryMu.Lock()
	m.runRuntimeCommandsReady = false
	m.startupRecoveryActions = nil
	m.startupRecoveryMu.Unlock()
	m.started = false
	// Recovery may have accepted earlier actions before a later item failed.
	// Clear process-local gates only when no such owner work is in flight. Never
	// turn a startup error into an unbounded teardown wait: a busy cancelled
	// lifecycle remains closed to Start and a later deadline-aware Stop drains it.
	if !m.lifecycleMu.TryLock() {
		return
	}
	defer m.lifecycleMu.Unlock()
	m.runtimeMutationFence.Lock()
	m.driveMu.Lock()
	m.lifecycleCtx = nil
	m.lifecycleCancel = nil
	m.runtimeLeaseTimerCtx = nil
	m.runtimeLeaseTimerCancel = nil
	m.drivingRuns = nil
	m.persistedLeaseMounts = nil
	m.retryLeaseTimers = nil
	m.retiredWorkspaces = nil
	m.quiescingWorkspaces = nil
	m.driveMu.Unlock()
	m.runtimeMutationFence.Unlock()
}

func (m *Biz) unsubscribeRuntimeIngress() {
	if m == nil {
		return
	}
	for idx := len(m.rawTopics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.rawTopics[idx])
	}
	m.UnsubscribeMatchedFunc(ModuleID+"/run-drive", runevents.TopicRunStateChanged)
	m.UnsubscribeMatchedFunc(ModuleID+"/run-drive", runevents.TopicRunContinuationRequested)
	m.UnsubscribeMatchedFunc(ModuleID+"/run-drive", runevents.TopicRunRuntimeCommandsReady)
	m.rawTopics = nil
}

func (m *Biz) lockLifecycleForStop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		return common.ErrInvalidArgument
	}
	if m.lifecycleMu.TryLock() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if m.lifecycleMu.TryLock() {
				return nil
			}
		}
	}
}

func (m *Biz) lockLifecycleTransition(ctx context.Context) error {
	if m == nil || ctx == nil {
		return common.ErrInvalidArgument
	}
	if m.lifecycleStopMu.TryLock() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if m.lifecycleStopMu.TryLock() {
				return nil
			}
		}
	}
}

func (m *Biz) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		return common.ErrInvalidArgument
	}
	if err := m.lockLifecycleTransition(ctx); err != nil {
		return err
	}
	defer m.lifecycleStopMu.Unlock()
	// Wake every delayed persisted-lease timer before waiting for lifecycle
	// readers. The lease remains persisted; cancellation removes only local
	// delivery machinery and fresh startup will mount it again.
	m.driveMu.Lock()
	if !m.runtimeLeaseTimerClosing {
		m.runtimeLeaseTimerClosing = true
		m.runtimeLeaseTimerEpoch++
	}
	if m.lifecycleCancel != nil {
		m.lifecycleCancel()
	}
	if m.runtimeLeaseTimerCancel != nil {
		m.runtimeLeaseTimerCancel()
	}
	m.driveMu.Unlock()
	m.started = false
	// Remove every external ingress before waiting. Already-running handlers hold
	// lifecycleMu; queued drive/timer callbacks observe the cancelled lifecycle
	// root and the closing epoch before they can create another successor.
	m.unsubscribeRuntimeIngress()
	// A drive can still be executing after its EventHub subscription has been
	// removed. Wait for it before dropping the owner dependencies it uses; this
	// is lifecycle synchronization only, not a second recovery state machine.
	if err := m.lockLifecycleForStop(ctx); err != nil {
		return err
	}
	defer m.lifecycleMu.Unlock()
	m.runtimeController = nil
	m.evidence = RuntimeEvidenceRepositories{}
	m.evidenceRecords = nil
	m.workspaceDeletions = nil
	m.workspacePurger = nil
	m.runtimeMutationFence.Lock()
	m.driveMu.Lock()
	// Cancel again under the lifecycle write fence. A pre-existing reader may
	// have captured the prior context before the first cancel point, but the
	// closing+epoch gate has forbidden it from publishing any new context or
	// background timer since that point.
	if m.runtimeLeaseTimerCancel != nil {
		m.runtimeLeaseTimerCancel()
	}
	m.drivingRuns = nil
	m.persistedLeaseMounts = nil
	m.retryLeaseTimers = nil
	m.retiredWorkspaces = nil
	m.quiescingWorkspaces = nil
	m.runtimeLeaseTimerCtx = nil
	m.runtimeLeaseTimerCancel = nil
	m.lifecycleCtx = nil
	m.lifecycleCancel = nil
	m.driveMu.Unlock()
	m.runtimeMutationFence.Unlock()
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
	if !ok || m == nil || ev.Source() != runevents.CommandDestination || data.Validate() != nil || data.State != common.RunStateActive {
		return
	}
	// A notification may already be queued when Stop removes the matched
	// observer. Fence validation, admission, and task submission as one in-flight
	// lifecycle operation so teardown cannot clear dependencies in between.
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	if m.runtimeController == nil {
		return
	}
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

func (m *Biz) handleRunContinuationRequested(ev magicevent.Event, _ magicevent.Result) {
	data, ok := ev.Data().(runevents.RunContinuationRequestedData)
	if !ok || m == nil || ev.Source() != runevents.CommandDestination || data.Validate() != nil {
		return
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	if m.runtimeController == nil {
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
	recoveryCtx := m.orchestratorBackgroundContext(ctx, workspaceID)
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
	switch ev.ID() {
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
	if err := m.runtimeLifecycleSchedulingError(); err != nil {
		return err
	}
	if err := m.validateWorkspaceRetirementDriveAdmission(ctx, workspaceID, runID, leaseGeneration); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	m.runtimeMutationFence.RLock()
	_, quiescing := m.quiescingWorkspaces[workspaceID]
	m.runtimeMutationFence.RUnlock()
	if quiescing {
		return common.ErrConflict
	}
	driveKey := runtimeDriveKey(workspaceID, runID)
	if leaseGeneration == 0 && !m.beginRuntimeDrive(driveKey) {
		return nil
	}
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: workspaceID, RunID: runID, Kind: orchestratorevents.RuntimeDriveObservationScheduled,
		RecoveryGeneration: leaseGeneration, Source: source,
	})
	request := scheduledRuntimeDriveRequest{
		workspaceID: workspaceID, runID: runID, source: source, driveKey: driveKey, leaseGeneration: leaseGeneration,
		reconciliationRequestID: reconciliationRequestID, reconciliationGeneration: reconciliationGeneration,
	}
	bgCtx := m.orchestratorBackgroundContext(ctx, workspaceID)
	// A timer already runs inside BackgroundRoutine. Execute the claimed lease
	// there so the persisted running lease and local gate have no handoff gap.
	if m.BackgroundRoutine() == nil || leaseGeneration != 0 {
		m.runScheduledRuntimeDrive(bgCtx, request)
		return nil
	}
	if err := m.BackgroundRoutine().AsyncFunction(func() { m.runScheduledRuntimeDrive(bgCtx, request) }); err != nil {
		m.endRuntimeDrive(driveKey)
		return err
	}
	return nil
}

type scheduledRuntimeDriveRequest struct {
	workspaceID              common.WorkspaceID
	runID                    common.RunID
	source                   string
	driveKey                 string
	leaseGeneration          uint64
	reconciliationRequestID  string
	reconciliationGeneration uint64
}

func (m *Biz) runScheduledRuntimeDrive(ctx context.Context, request scheduledRuntimeDriveRequest) {
	var retry *runtimeRecoveryLeaseRequest
	m.lifecycleMu.RLock()
	if m.runtimeController == nil || m.EventHub() == nil {
		m.lifecycleMu.RUnlock()
		m.endRuntimeDrive(request.driveKey)
		return
	}
	defer m.lifecycleMu.RUnlock()
	// This outer fence covers panic inside the exit/lease-CAS machinery itself.
	// It never recursively calls finish: after a safe load it mounts existing
	// durable authority or makes at most one owner-private successor transition.
	defer func() {
		if recovered := recover(); recovered != nil {
			m.recoverScheduledRuntimeDriveExitPanic(ctx, request, recovered, retry)
		}
	}()
	finishRequired := false
	// Install the owner exit fence before even the persisted-lease preflight:
	// repository/admission code is part of this background worker and can panic
	// before the ordinary drive body begins. A successful persisted-lease adopt
	// completes its own gate handoff and therefore leaves finishRequired false.
	defer func() {
		if finishRequired {
			m.finishScheduledRuntimeDrive(ctx, request, &retry)
		}
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			panicFailure, _ := m.runtimeScheduledDrivePanicFailure(ctx, request, recovered)
			retry = m.handleScheduledRuntimeDriveFailure(ctx, request, panicFailure)
			if retry == nil {
				// The panic branch is deliberately allocation-free apart from this
				// value. Keep a stable fallback so a future classifier edit cannot
				// accidentally turn a recovered panic into a generation-zero gate
				// release without persisted successor authority.
				retry = runtimeScheduledDrivePanicLeaseRequest(request.source, 0, false)
			}
			finishRequired = true
		}
	}()
	if request.leaseGeneration == 0 {
		// A Start/Resume notification is not authority to bypass an existing
		// persisted lease. When a prior local scheduler disappeared (for example
		// after a renewal-CAS or timer-acceptance failure), adopt that exact SM-4
		// authority or fail closed before any provider/capability effect.
		handled, err := m.adoptPersistedRuntimeDriveLeaseForNormalDrive(ctx, request)
		if err != nil {
			retry = m.handleScheduledRuntimeDriveFailure(ctx, request, err)
			if retry == nil {
				retry = runtimePersistedLeasePreflightRequest(request.source, runtimePersistedLeasePreflightPointLoad)
			}
			finishRequired = true
			return
		}
		if handled {
			return
		}
	}
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: request.workspaceID, RunID: request.runID, Kind: orchestratorevents.RuntimeDriveObservationRunning,
		RecoveryGeneration: request.leaseGeneration, LeaseState: runtimeDriveLeaseStateForGeneration(request.leaseGeneration), Source: request.source,
	})
	finishRequired = true
	err := m.driveAndProjectRuntimeLockedWithCommand(ctx, orchestratorevents.DriveRuntimeCommand{
		WorkspaceID: request.workspaceID, RunID: request.runID,
		RecoveryLeaseGeneration: request.leaseGeneration,
		ReconciliationRequestID: request.reconciliationRequestID, ReconciliationGeneration: request.reconciliationGeneration,
	})
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	retry = m.handleScheduledRuntimeDriveFailure(ctx, request, err)
}

func (m *Biz) finishScheduledRuntimeDrive(ctx context.Context, request scheduledRuntimeDriveRequest, retry **runtimeRecoveryLeaseRequest) {
	retryPending := *retry != nil
	if *retry != nil && request.leaseGeneration != 0 {
		// Only the worker that claimed this exact generation may replace its
		// running lease. Carry the generation into the owner-private scheduling
		// event so repeated delivery failure cannot collapse into a running
		// duplicate that is then released without a successor.
		(*retry).FromRunningGeneration = request.leaseGeneration
	}
	if *retry != nil && !runtimeRetryUsesExistingPersistedLease(**retry) && m.scheduleRuntimeRecoveryLease(ctx, request.workspaceID, request.runID, **retry) {
		return
	}
	if retryPending {
		if !m.recoverAndMountRuntimeDriveLeaseAfterScheduleFailure(ctx, request) {
			// A claimed generation retains its persisted running record; a
			// generation-zero drive with no successful lease CAS retains only this
			// process-local exclusion gate until Stop. In both cases fresh startup
			// reclassifies the active aggregate. Releasing here would allow an
			// ordinary ingress to become a concurrent second drive without a
			// durable successor.
			slog.Warn("workorch Runtime retaining drive gate after recovery lease persistence or mount failure", "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "source", request.source)
		}
		return
	}
	// A failed renewal CAS must not be followed by release of the running
	// generation: that generation is the only durable recovery authority left.
	// If persistence succeeded but the local timer was rejected, the replacement
	// scheduled generation is likewise left intact for fresh-start recovery.
	if request.leaseGeneration != 0 {
		if _, err := m.runtimeReleaseDriveLease(ctx, request.workspaceID, request.runID, request.leaseGeneration); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch Runtime drive lease release failed", "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "err", err)
		}
	}
	// A governance-resolution command can persist an exact lifecycle delivery
	// lease while this worker still owns the run-scoped gate. The command never
	// starts a timer behind the gate; instead this sole gate-owner exit point
	// atomically retains the gate and mounts the persisted generation. If the
	// command won the race after this exit, it acquires the now-free gate itself.
	m.mountPersistedRuntimeDriveLeaseAtGateExit(ctx, request)
}

// adoptPersistedRuntimeDriveLeaseForNormalDrive is the generation-zero
// boundary guard. It runs only inside the already-serialized background
// worker, avoiding synchronous EventHub re-entry from RunStateChanged. A
// scheduled lease is mounted as-is; a running lease with no process-local gate
// owner is treated as interrupted and owner-locally recovered before mounting.
func (m *Biz) adoptPersistedRuntimeDriveLeaseForNormalDrive(ctx context.Context, request scheduledRuntimeDriveRequest) (bool, error) {
	aggregate, err := m.runtimeController.Load(ctx, request.workspaceID, request.runID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return false, nil
		}
		slog.Warn("workorch Runtime normal drive preflight could not inspect persisted lease", "workspaceID", request.workspaceID, "runID", request.runID, "source", request.source, "err", err)
		return false, &runtimePersistedLeasePreflightError{Point: runtimePersistedLeasePreflightPointLoad, Cause: err}
	}
	if aggregate.DriveLease == nil {
		return false, nil
	}
	lease := aggregate.DriveLease.Clone()
	if lease.State == runtimecontract.RuntimeDriveLeaseRunning {
		lease, _, err = m.runtimeRecoverDriveLease(ctx, request.workspaceID, request.runID, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			slog.Warn("workorch Runtime normal drive could not recover persisted running lease", "workspaceID", request.workspaceID, "runID", request.runID, "source", request.source, "generation", aggregate.DriveLease.RecoveryGeneration, "err", err)
			return false, &runtimePersistedLeasePreflightError{Point: runtimePersistedLeasePreflightPointRecover, Cause: err}
		}
	}
	if lease.State != runtimecontract.RuntimeDriveLeaseScheduled || !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, request.source+":persisted_lease_adopted") {
		slog.Warn("workorch Runtime normal drive could not mount persisted lease", "workspaceID", request.workspaceID, "runID", request.runID, "source", request.source, "generation", lease.RecoveryGeneration, "state", lease.State)
		return false, &runtimePersistedLeasePreflightError{Point: runtimePersistedLeasePreflightPointMount, Cause: common.ErrConflict}
	}
	m.clearPersistedRuntimeDriveLeaseMount(request.driveKey, lease.RecoveryGeneration)
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: request.source + ":persisted_lease_adopted",
	})
	return true, nil
}

// recoverAndMountRuntimeDriveLeaseAfterScheduleFailure preserves whichever
// successor is already durable when replacement persistence or local timer
// mounting failed. For a claimed generation, recovery is another
// generation-fenced owner-private CAS. For a generation-zero worker it adopts
// an already scheduled lease without manufacturing another drive authority.
// If neither is possible, the caller deliberately retains the process gate for
// Stop/fresh-start recovery.
func (m *Biz) recoverAndMountRuntimeDriveLeaseAfterScheduleFailure(ctx context.Context, request scheduledRuntimeDriveRequest) bool {
	lease, _, err := m.runtimeRecoverDriveLease(ctx, request.workspaceID, request.runID, time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano))
	if err != nil {
		slog.Warn("workorch Runtime lease recovery after schedule failure failed", "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "source", request.source, "err", err)
		return false
	}
	if lease.State != runtimecontract.RuntimeDriveLeaseScheduled || !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, request.source+":renewal_recovered") {
		slog.Warn("workorch Runtime recovered lease timer was not accepted", "workspaceID", request.workspaceID, "runID", request.runID, "generation", lease.RecoveryGeneration, "source", request.source)
		return false
	}
	m.clearPersistedRuntimeDriveLeaseMount(request.driveKey, lease.RecoveryGeneration)
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: request.source + ":renewal_recovered",
	})
	return true
}

// runtimeScheduledDrivePanicError is owner-local and deliberately excludes the
// recovered value. Panic text is neither stable identity nor Runtime authority;
// the persisted aggregate revision is the only fact that may bind re-drive.
type runtimeScheduledDrivePanicError struct {
	PersistedRevision uint64
	RevisionKnown     bool
}

func (e *runtimeScheduledDrivePanicError) Error() string {
	if e == nil || !e.RevisionKnown {
		return "Runtime scheduled drive panicked after an unknown persisted revision"
	}
	return fmt.Sprintf("Runtime scheduled drive panicked after persisted revision %d", e.PersistedRevision)
}

func (m *Biz) runtimeScheduledDrivePanicFailure(ctx context.Context, request scheduledRuntimeDriveRequest, recovered any) (*runtimeScheduledDrivePanicError, runtime.RuntimeAggregate) {
	failure := &runtimeScheduledDrivePanicError{}
	aggregate, err := m.loadRuntimeAggregateAfterScheduledDrivePanic(ctx, request.workspaceID, request.runID)
	if err == nil {
		failure.PersistedRevision = aggregate.StateRevision
		failure.RevisionKnown = true
	}
	slog.Error("workorch Runtime scheduled drive panic recovered by owner", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "persistedRevision", failure.PersistedRevision, "revisionKnown", failure.RevisionKnown, "panicType", fmt.Sprintf("%T", recovered), "loadErr", err)
	return failure, aggregate
}

// recoverScheduledRuntimeDriveExitPanic is the bounded outer exit fence. It is
// intentionally non-recursive: one safe load, then either mount the exact
// durable scheduled generation, recover the exact running authority through
// the existing private event and mount it, or make at most one private schedule
// attempt when no lease survived. Any secondary panic/failure retains the gate
// and leaves the durable aggregate for startup recovery.
func (m *Biz) recoverScheduledRuntimeDriveExitPanic(ctx context.Context, request scheduledRuntimeDriveRequest, recovered any, retry *runtimeRecoveryLeaseRequest) {
	m.beginRuntimeDrive(request.driveKey)
	defer func() {
		if secondary := recover(); secondary != nil {
			m.beginRuntimeDrive(request.driveKey)
			slog.Error("workorch Runtime scheduled drive exit recovery panic retained owner gate", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "panicType", fmt.Sprintf("%T", secondary))
		}
	}()
	failure, aggregate := m.runtimeScheduledDrivePanicFailure(ctx, request, recovered)
	if failure.RevisionKnown && aggregate.DriveLease != nil {
		lease := aggregate.DriveLease.Clone()
		if lease.State == runtimecontract.RuntimeDriveLeaseRunning {
			runningGeneration := lease.RecoveryGeneration
			var err error
			lease, _, err = m.runtimeRecoverDriveLease(ctx, request.workspaceID, request.runID, time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano))
			if err != nil {
				slog.Warn("workorch Runtime exit panic could not recover exact running lease", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "generation", aggregate.DriveLease.RecoveryGeneration, "err", err)
				return
			}
			if lease.State != runtimecontract.RuntimeDriveLeaseScheduled || lease.RecoveryGeneration != runningGeneration+1 {
				slog.Warn("workorch Runtime exit panic running lease recovery lost exact generation", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "expectedGeneration", runningGeneration+1, "observedGeneration", lease.RecoveryGeneration, "state", lease.State)
				return
			}
		}
		m.mountScheduledRuntimeDriveLeaseAfterExitPanic(ctx, request, lease)
		return
	}
	if retry == nil && failure.RevisionKnown &&
		(runtimeTerminalDeliveryFullyAcknowledged(aggregate) || aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance) {
		m.endRuntimeDrive(request.driveKey)
		return
	}
	requestLease := retry
	if requestLease == nil {
		requestLease = runtimeScheduledDrivePanicLeaseRequest(request.source, failure.PersistedRevision, failure.RevisionKnown)
	}
	if requestLease == nil || requestLease.Reason == "" || requestLease.FailureSignature == "" {
		return
	}
	lease, _, err := m.runtimeScheduleDriveLease(ctx, request.workspaceID, request.runID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: request.workspaceID, RunID: request.runID,
		Reason: requestLease.Reason, FailureSignature: requestLease.FailureSignature,
		DueAt: time.Now().UTC().Add(max(requestLease.Delay, time.Second)).Format(time.RFC3339Nano),
	}, 0)
	if err != nil {
		slog.Warn("workorch Runtime exit panic bounded successor schedule failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		return
	}
	m.mountScheduledRuntimeDriveLeaseAfterExitPanic(ctx, request, lease)
}

func (m *Biz) mountScheduledRuntimeDriveLeaseAfterExitPanic(ctx context.Context, request scheduledRuntimeDriveRequest, lease runtimecontract.RuntimeDriveLease) bool {
	if lease.Validate() != nil || lease.WorkspaceID != request.workspaceID || lease.RunID != request.runID || lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return false
	}
	if !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, request.source+":exit_panic_durable_successor") {
		slog.Warn("workorch Runtime exit panic retained unmounted durable lease", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "generation", lease.RecoveryGeneration)
		return false
	}
	m.clearPersistedRuntimeDriveLeaseMount(request.driveKey, lease.RecoveryGeneration)
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: request.source + ":exit_panic_durable_successor",
	})
	return true
}

// loadRuntimeAggregateAfterScheduledDrivePanic is itself panic-safe because a
// repository fault may have caused the original panic. If the owner repository
// remains unavailable, finishScheduledRuntimeDrive still receives a non-nil
// retry and retains the gate when it cannot persist a successor.
func (m *Biz) loadRuntimeAggregateAfterScheduledDrivePanic(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (aggregate runtime.RuntimeAggregate, err error) {
	defer func() {
		if recover() != nil {
			aggregate = runtime.RuntimeAggregate{}
			err = fmt.Errorf("Runtime aggregate repository panicked during scheduled-drive recovery")
		}
	}()
	return m.runtimeController.Load(ctx, workspaceID, runID)
}

func runtimeScheduledDrivePanicLeaseRequest(source string, revision uint64, revisionKnown bool) *runtimeRecoveryLeaseRequest {
	signature := "persisted-pending/unknown"
	if revisionKnown {
		signature = fmt.Sprintf("persisted-pending/%d", revision)
	}
	return &runtimeRecoveryLeaseRequest{
		Reason:           runtimecontract.RuntimeDriveLeaseReasonPersistedPending,
		FailureSignature: signature,
		Delay:            time.Second,
		Source:           source,
	}
}

type runtimePersistedLeasePreflightPoint string

const (
	runtimePersistedLeasePreflightPointLoad    runtimePersistedLeasePreflightPoint = "load"
	runtimePersistedLeasePreflightPointRecover runtimePersistedLeasePreflightPoint = "recover"
	runtimePersistedLeasePreflightPointMount   runtimePersistedLeasePreflightPoint = "mount"
)

type runtimePersistedLeasePreflightError struct {
	Point runtimePersistedLeasePreflightPoint
	Cause error
}

func (e *runtimePersistedLeasePreflightError) Error() string {
	if e == nil {
		return "Runtime persisted lease preflight failed"
	}
	return fmt.Sprintf("Runtime persisted lease preflight %s failed", e.Point)
}

func (e *runtimePersistedLeasePreflightError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func runtimePersistedLeasePreflightRequest(source string, point runtimePersistedLeasePreflightPoint) *runtimeRecoveryLeaseRequest {
	if point == "" {
		point = runtimePersistedLeasePreflightPointLoad
	}
	return &runtimeRecoveryLeaseRequest{
		Reason: runtimecontract.RuntimeDriveLeaseReasonScheduleRetry, FailureSignature: "persisted-lease-preflight/" + string(point), Delay: time.Second, Source: source,
	}
}

func runtimeRetryUsesExistingPersistedLease(request runtimeRecoveryLeaseRequest) bool {
	return request.Reason == runtimecontract.RuntimeDriveLeaseReasonScheduleRetry &&
		(request.FailureSignature == "persisted-lease-preflight/"+string(runtimePersistedLeasePreflightPointRecover) ||
			request.FailureSignature == "persisted-lease-preflight/"+string(runtimePersistedLeasePreflightPointMount))
}

func (m *Biz) handleScheduledRuntimeDriveFailure(ctx context.Context, request scheduledRuntimeDriveRequest, err error) *runtimeRecoveryLeaseRequest {
	var preflight *runtimePersistedLeasePreflightError
	if errors.As(err, &preflight) {
		return runtimePersistedLeasePreflightRequest(request.source, preflight.Point)
	}
	var panicFailure *runtimeScheduledDrivePanicError
	if errors.As(err, &panicFailure) {
		return runtimeScheduledDrivePanicLeaseRequest(request.source, panicFailure.PersistedRevision, panicFailure.RevisionKnown)
	}
	var recoverableWait *runtimeRecoverableWaitPendingError
	if errors.As(err, &recoverableWait) {
		delay := time.Second
		if dueAt, parseErr := time.Parse(time.RFC3339Nano, recoverableWait.Wait.NextEligibleAt); parseErr == nil {
			delay = time.Until(dueAt)
			if delay <= 0 {
				delay = time.Millisecond
			}
		}
		return &runtimeRecoveryLeaseRequest{
			Reason:           runtimecontract.RuntimeDriveLeaseReasonRecoverableWait,
			FailureSignature: "recoverable-wait/" + recoverableWait.Wait.WaitID,
			Delay:            delay,
			Source:           request.source,
		}
	}
	if operationID, claimID, deadlineText, inFlight := runtime.RuntimeOperationRecoveryInFlight(err); inFlight {
		deadline, parseErr := time.Parse(time.RFC3339Nano, deadlineText)
		if parseErr != nil {
			return nil
		}
		delay := time.Until(deadline)
		if delay <= 0 {
			delay = time.Millisecond
		}
		return &runtimeRecoveryLeaseRequest{
			Reason:           runtimecontract.RuntimeDriveLeaseReasonOperationRetry,
			FailureSignature: "operation-recovery-in-flight/" + operationID + "/" + claimID,
			Delay:            delay, Source: request.source,
		}
	}
	var persisted *runtimePersistedDrivePendingError
	if errors.As(err, &persisted) {
		slog.Debug("workorch Runtime persisted bounded retry state", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "revision", persisted.Revision)
		return &runtimeRecoveryLeaseRequest{Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: fmt.Sprintf("persisted-pending/%d", persisted.Revision), Delay: time.Second, Source: request.source}
	}
	var lifecycleDeliveryErr *runtimeLifecycleOwnerDeliveryError
	if errors.As(err, &lifecycleDeliveryErr) {
		return m.handleRuntimeLifecycleOwnerDeliveryRetry(ctx, request, lifecycleDeliveryErr)
	}
	var terminalDeliveryErr *runtimeTerminalDeliveryError
	if errors.As(err, &terminalDeliveryErr) {
		return m.handleRuntimeTerminalDeliveryRetry(ctx, request, err)
	}
	slog.Warn("workorch Runtime drive failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
	failure := classifyRuntimeDriveFailure(err)
	m.observeScheduledRuntimeDriveFailure(ctx, request, failure, err)
	if !runtime.FailureClassIsBusinessTerminal(failure.Class) {
		return m.handleRuntimeNonTerminalRetry(ctx, request, failure, err)
	}
	return m.handleRuntimeBusinessTerminalFailure(ctx, request, failure)
}

func (m *Biz) handleRuntimeLifecycleOwnerDeliveryRetry(ctx context.Context, request scheduledRuntimeDriveRequest, deliveryErr *runtimeLifecycleOwnerDeliveryError) *runtimeRecoveryLeaseRequest {
	signature := deliveryErr.leaseSignature()
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: request.workspaceID, RunID: request.runID, Kind: orchestratorevents.RuntimeDriveObservationDeliveryPending,
		RecoveryGeneration: request.leaseGeneration, FailureCategory: "lifecycle_owner_delivery_pending", LeaseState: runtimeDriveLeaseStateForGeneration(request.leaseGeneration), Source: request.source,
	})
	slog.Warn("workorch Runtime lifecycle owner delivery failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "signature", signature, "err", deliveryErr)
	// Governance wait is nonterminal. Persist the exact handoff as an SM-4
	// DriveLease and keep retry authority alive until Run owner accepts it (or a
	// fresh process recovers the same lease). It must never consume the terminal
	// Result/Workspace/Run delivery budget.
	return runtimeDeliveryRecoveryLease(request.source, signature)
}

// scheduleRuntimeLifecycleOwnerDeliveryRetry enters a continuation failure
// that happened outside a scheduled drive (the governance-resolution command
// path) into the same persisted SM-4 lease ledger. The lease is authority; the
// process-local timer and singleflight gate are only delivery optimizations.
// No terminal-delivery or operation-recovery budget is consulted or consumed.
func (m *Biz) scheduleRuntimeLifecycleOwnerDeliveryRetry(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string, deliveryErr *runtimeLifecycleOwnerDeliveryError) error {
	if m == nil || deliveryErr == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	request := runtimeDeliveryRecoveryLease(source, deliveryErr.leaseSignature())
	if request == nil {
		return common.ErrInvalidArgument
	}
	lease, _, err := m.runtimeScheduleDriveLease(ctx, workspaceID, runID, runtimecontract.RuntimeDriveLease{
		WorkspaceID: workspaceID, RunID: runID, Reason: request.Reason, FailureSignature: request.FailureSignature,
		DueAt: time.Now().UTC().Add(request.Delay).Format(time.RFC3339Nano),
	}, 0)
	if err != nil {
		return err
	}
	return m.mountPersistedRuntimeLifecycleOwnerDeliveryLease(ctx, lease, source, request.FailureSignature)
}

// mountPersistedRuntimeLifecycleOwnerDeliveryLease attaches delivery machinery
// to an already-authoritative scheduled lease. It never creates or rewrites
// retry authority. If another drive owns the run-scoped gate, the exact
// persisted generation is latched for that owner's sole exit path.
func (m *Biz) mountPersistedRuntimeLifecycleOwnerDeliveryLease(
	ctx context.Context,
	lease runtimecontract.RuntimeDriveLease,
	source string,
	expectedSignature string,
) error {
	ownsGate, err := m.retainRuntimeLifecycleOwnerDeliveryLeaseGate(lease, expectedSignature)
	if err != nil || !ownsGate {
		return err
	}
	return m.mountRetainedRuntimeLifecycleOwnerDeliveryLease(ctx, lease, source)
}

// retainRuntimeLifecycleOwnerDeliveryLeaseGate is called before a governance
// continuation is delivered to Run. It prevents RunStateChanged(active) from
// starting a normal drive between the owner acknowledgement and mounting the
// already-persisted successor. A false result means the exact lease was
// latched behind an older gate owner (or is already running).
func (m *Biz) retainRuntimeLifecycleOwnerDeliveryLeaseGate(lease runtimecontract.RuntimeDriveLease, expectedSignature string) (bool, error) {
	if m == nil || lease.Validate() != nil || lease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry ||
		lease.FailureSignature != expectedSignature {
		return false, fmt.Errorf("%w: a different Runtime drive lease already owns lifecycle delivery", common.ErrConflict)
	}
	if lease.State == runtimecontract.RuntimeDriveLeaseRunning {
		return false, nil
	}
	if lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return false, fmt.Errorf("%w: persisted lifecycle delivery lease is not scheduled", common.ErrConflict)
	}
	key := runtimeDriveKey(lease.WorkspaceID, lease.RunID)
	ownsGate := m.beginRuntimeDrive(key)
	if !ownsGate {
		// Never let a timer bypass a live drive gate. Atomically hand the exact
		// persisted generation to that gate owner; if the old worker already
		// exited, this caller takes the free gate and mounts the timer itself.
		ownsGate = m.retainRuntimeDriveGateForPersistedLease(key, lease)
	}
	return ownsGate, nil
}

func (m *Biz) mountRetainedRuntimeLifecycleOwnerDeliveryLease(ctx context.Context, lease runtimecontract.RuntimeDriveLease, source string) error {
	key := runtimeDriveKey(lease.WorkspaceID, lease.RunID)
	if !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, source) {
		m.endRuntimeDrive(key)
		return fmt.Errorf("%w: persisted lifecycle delivery lease timer was not accepted", common.ErrConflict)
	}
	m.clearPersistedRuntimeDriveLeaseMount(key, lease.RecoveryGeneration)
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: source,
	})
	return nil
}

func (m *Biz) handleRuntimeTerminalDeliveryRetry(ctx context.Context, request scheduledRuntimeDriveRequest, err error) *runtimeRecoveryLeaseRequest {
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: request.workspaceID, RunID: request.runID, Kind: orchestratorevents.RuntimeDriveObservationDeliveryPending,
		RecoveryGeneration: request.leaseGeneration, FailureCategory: "terminal_delivery_pending", LeaseState: runtimeDriveLeaseStateForGeneration(request.leaseGeneration), Source: request.source,
	})
	slog.Warn("workorch Runtime terminal delivery failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
	if m.consumeRuntimeDeliveryRetryBudget(ctx, request) {
		return runtimeDeliveryRecoveryLease(request.source, "terminal-delivery")
	}
	return nil
}

func (m *Biz) observeScheduledRuntimeDriveFailure(ctx context.Context, request scheduledRuntimeDriveRequest, failure runtimecontract.FailureFact, err error) {
	revision, observed := runtimeDriveFailureObservedRevision(err)
	if !observed {
		return
	}
	kind := orchestratorevents.RuntimeDriveObservationCASReload
	if failure.Code != "cas_conflict" {
		kind = orchestratorevents.RuntimeDriveObservationSemanticRejection
	}
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: request.workspaceID, RunID: request.runID, Kind: kind, AggregateRevision: revision,
		RecoveryGeneration: request.leaseGeneration, FailureCategory: failure.Code, LeaseState: runtimeDriveLeaseStateForGeneration(request.leaseGeneration), Source: request.source,
	})
}

func (m *Biz) handleRuntimeNonTerminalRetry(ctx context.Context, request scheduledRuntimeDriveRequest, failure runtimecontract.FailureFact, driveErr error) *runtimeRecoveryLeaseRequest {
	slog.Warn("workorch Runtime drive non-terminal failure", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "class", failure.Class, "err", driveErr)
	budgetKind, ok := runtimeRecoveryBudgetKind(failure.Class)
	if !ok {
		return nil
	}
	_, consumed, err := m.runtimeConsumeRecoveryBudget(ctx, runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: request.workspaceID, RunID: request.runID}}, budgetKind)
	if err != nil {
		slog.Warn("workorch Runtime recovery budget commit failed", "workspaceID", request.workspaceID, "runID", request.runID, "kind", budgetKind, "err", err)
		return nil
	}
	if consumed {
		return runtimeLeaseRequestForFailure(failure, driveErr, request.source, time.Second)
	}
	if failure.Class == runtimecontract.FailureClassOwnerTransientUnavailable {
		return m.recordExhaustedOwnerTransient(ctx, request, failure)
	}
	if failure.Class == runtimecontract.FailureClassCASConflict {
		slog.Error("workorch Runtime CAS recovery ledger exhausted; keeping owner-local re-drive pending", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID)
		return runtimeLeaseRequestForFailure(failure, driveErr, request.source, 5*time.Second)
	}
	return nil
}

func (m *Biz) recordExhaustedOwnerTransient(ctx context.Context, request scheduledRuntimeDriveRequest, failure runtimecontract.FailureFact) *runtimeRecoveryLeaseRequest {
	_, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveFailureObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: request.workspaceID, RunID: request.runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{FailureFact: &failure},
	})
	if err != nil {
		slog.Warn("workorch Runtime exhausted operation recovery fact commit failed", "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		return runtimeFailureFactPendingLeaseRequest(request.source, failure, err)
	}
	return runtimeDeliveryRecoveryLease(request.source, "owner-transient-terminal")
}

func (m *Biz) handleRuntimeBusinessTerminalFailure(ctx context.Context, request scheduledRuntimeDriveRequest, failure runtimecontract.FailureFact) *runtimeRecoveryLeaseRequest {
	recorded, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveFailureObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: request.workspaceID, RunID: request.runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{FailureFact: &failure},
	})
	if err != nil {
		slog.Warn("workorch Runtime drive failure fact commit failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		return runtimeFailureFactPendingLeaseRequest(request.source, failure, err)
	}
	aggregate := recorded.Aggregate
	if aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || aggregate.Decision.Terminal == nil {
		slog.Warn("workorch Runtime drive failure did not reduce to terminal", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID)
		return runtimeFailureFactPendingLeaseRequest(request.source, failure, nil)
	}
	if err := m.deliverRuntimeTerminal(ctx, aggregate); err != nil {
		m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
			WorkspaceID: request.workspaceID, RunID: request.runID, Kind: orchestratorevents.RuntimeDriveObservationDeliveryPending,
			AggregateRevision: aggregate.StateRevision, RecoveryGeneration: request.leaseGeneration, FailureCategory: "terminal_delivery_pending", LeaseState: runtimeDriveLeaseStateForGeneration(request.leaseGeneration), Source: request.source,
		})
		slog.Warn("workorch Runtime drive failure delivery failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		if m.consumeRuntimeDeliveryRetryBudget(ctx, request) {
			return runtimeDeliveryRecoveryLease(request.source, "terminal-delivery")
		}
	}
	return nil
}

func (m *Biz) consumeRuntimeDeliveryRetryBudget(ctx context.Context, request scheduledRuntimeDriveRequest) bool {
	_, consumed, err := m.runtimeConsumeRecoveryBudget(ctx, runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: request.workspaceID, RunID: request.runID}}, runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
	if err != nil {
		slog.Warn("workorch Runtime terminal delivery budget commit failed", "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		return false
	}
	return consumed
}

func runtimeDeliveryRecoveryLease(source, signature string) *runtimeRecoveryLeaseRequest {
	return &runtimeRecoveryLeaseRequest{Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: signature, Delay: time.Second, Source: source}
}

// runtimeFailureFactPendingLeaseRequest preserves SM-4 successor authority
// when the owner-private failure transition has not reached its CAS boundary.
// This is not a delivery retry: no terminal authority exists until the exact
// failure fact and any active-dispatch invalidation commit together. A typed
// CAS conflict keeps its reload identity; every other persistence/validation
// failure is a stable persisted-pending re-drive that fresh startup can adopt.
func runtimeFailureFactPendingLeaseRequest(source string, failure runtimecontract.FailureFact, commitErr error) *runtimeRecoveryLeaseRequest {
	var cas *runtime.RuntimeCASConflictError
	if errors.As(commitErr, &cas) {
		return &runtimeRecoveryLeaseRequest{
			Reason: runtimecontract.RuntimeDriveLeaseReasonCASReload, FailureSignature: fmt.Sprintf("failure-fact-cas/%d/%d", cas.ExpectedRevision, cas.ObservedRevision),
			Delay: time.Second, Source: source,
		}
	}
	failure = failure.Normalized()
	identity := failure.Signature
	if identity == "" {
		identity = failure.Code
	}
	if identity == "" {
		identity = "unknown"
	}
	class := strings.TrimSpace(string(failure.Class))
	if class == "" {
		class = "unknown"
	}
	return &runtimeRecoveryLeaseRequest{
		Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "failure-fact-pending/" + class + "/" + identity,
		Delay: time.Second, Source: source,
	}
}

type runtimeRecoveryLeaseRequest struct {
	Reason                runtimecontract.RuntimeDriveLeaseReason
	FailureSignature      string
	Delay                 time.Duration
	Source                string
	FromRunningGeneration uint64
}

func runtimeDriveKey(workspaceID common.WorkspaceID, runID common.RunID) string {
	return string(workspaceID) + "\x00" + string(runID)
}

func runtimeDriveWorkspaceID(key string) common.WorkspaceID {
	workspaceID, _, ok := strings.Cut(key, "\x00")
	if !ok {
		return ""
	}
	return common.WorkspaceID(strings.TrimSpace(workspaceID))
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
	}, request.FromRunningGeneration)
	if err != nil {
		slog.Warn("workorch Runtime recovery lease commit failed", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "reason", request.Reason, "signature", request.FailureSignature, "err", err)
		return false
	}
	if lease.State != runtimecontract.RuntimeDriveLeaseScheduled || !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, request.Source) {
		slog.Warn("workorch Runtime recovery lease timer was not accepted", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "generation", lease.RecoveryGeneration)
		return false
	}
	m.clearPersistedRuntimeDriveLeaseMount(runtimeDriveKey(workspaceID, runID), lease.RecoveryGeneration)
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
	if err := m.validateWorkspaceRetirementLeaseTimerAdmission(ctx, lease); err != nil {
		return false
	}
	key := runtimeDriveKey(lease.WorkspaceID, lease.RunID)
	registered, accepted, epoch := m.beginRuntimeLeaseTimer(key, lease.RecoveryGeneration)
	if !accepted {
		return false
	}
	if !registered {
		return true
	}
	launched := false
	// BackgroundRoutine submission may reject or panic after the exact local
	// generation was registered. Clear that registration on every non-success
	// exit so the outer owner fence can remount the same durable lease instead of
	// mistaking an orphaned registration for a live timer.
	defer func() {
		if !launched {
			m.endRuntimeLeaseTimer(key, lease.RecoveryGeneration, epoch)
		}
	}()
	if !m.launchRuntimeDriveLeaseTimer(ctx, lease, source, epoch) {
		return false
	}
	launched = true
	return true
}

// launchRuntimeDriveLeaseTimer starts local delivery for an already-registered
// persisted generation. Every exit which did not transfer the gate into a
// claimed drive atomically consumes a newer persisted mount or releases the
// gate. This includes stale/duplicate claim results and timer/claim failures;
// a timer is never allowed to leave a mount behind a leaked gate.
func (m *Biz) launchRuntimeDriveLeaseTimer(_ context.Context, lease runtimecontract.RuntimeDriveLease, source string, epoch uint64) bool {
	if m == nil || m.BackgroundRoutine() == nil || lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return false
	}
	key := runtimeDriveKey(lease.WorkspaceID, lease.RunID)
	// Context publication is fenced by Stop's closing+epoch transition. Never
	// hold driveMu (or another Stop fence) across BackgroundRoutine submission:
	// a bounded task queue is allowed to block its caller.
	m.driveMu.Lock()
	if m.runtimeLeaseTimerClosing || epoch == 0 || epoch != m.runtimeLeaseTimerEpoch {
		m.driveMu.Unlock()
		return false
	}
	if m.runtimeLeaseTimerCtx == nil {
		root := m.lifecycleCtx
		if root == nil {
			root = context.Background()
		}
		m.runtimeLeaseTimerCtx, m.runtimeLeaseTimerCancel = context.WithCancel(root)
	}
	retryCtx := m.runtimeLeaseTimerCtx
	if lease.WorkspaceID != "" {
		retryCtx, _ = common.BindRuntimeWorkspaceContext(retryCtx, lease.WorkspaceID)
	}
	m.driveMu.Unlock()
	err := m.BackgroundRoutine().AsyncFunction(func() {
		gateTransferred := false
		stopCanceled := false
		var authoritativeSuccessor *runtimecontract.RuntimeDriveLease
		defer func() {
			if gateTransferred {
				m.endRuntimeLeaseTimer(key, lease.RecoveryGeneration, epoch)
				return
			}
			if stopCanceled {
				m.releaseCanceledRuntimeDriveLeaseTimer(key, lease.RecoveryGeneration, epoch)
				return
			}
			m.finishRuntimeDriveLeaseTimerAtGateExit(retryCtx, lease, authoritativeSuccessor, source, epoch)
		}()
		if !m.runtimeDriveLeaseTimerCallbackActive(retryCtx, epoch) {
			stopCanceled = true
			return
		}
		dueAt, parseErr := time.Parse(time.RFC3339Nano, lease.DueAt)
		if parseErr != nil {
			return
		}
		if delay := time.Until(dueAt); delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-retryCtx.Done():
				stopCanceled = true
				return
			case <-timer.C:
			}
		}
		if !m.runtimeDriveLeaseTimerCallbackActive(retryCtx, epoch) {
			stopCanceled = true
			return
		}
		m.observeRuntimeDrive(retryCtx, orchestratorevents.RuntimeDriveObservedData{
			WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerFired,
			AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: source,
		})
		claimedLease, claimed, claimErr := m.claimRuntimeDriveLeaseWhileStarted(retryCtx, lease, time.Now().UTC().Format(time.RFC3339Nano), epoch)
		if claimErr != nil {
			if errors.Is(claimErr, context.Canceled) {
				stopCanceled = true
				return
			}
			slog.Warn("workorch Runtime recovery lease claim failed", "source", source, "workspaceID", lease.WorkspaceID, "runID", lease.RunID, "generation", lease.RecoveryGeneration, "err", claimErr)
			return
		}
		if !claimed {
			if claimedLease.WorkspaceID != "" && claimedLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
				value := claimedLease.Clone()
				authoritativeSuccessor = &value
			}
			return
		}
		if err := m.scheduleRuntimeDriveWithLease(retryCtx, claimedLease.WorkspaceID, claimedLease.RunID, "recovery_lease:"+source, claimedLease.RecoveryGeneration); err != nil {
			if errors.Is(err, context.Canceled) {
				stopCanceled = true
				return
			}
			slog.Warn("workorch Runtime claimed recovery drive scheduling failed", "workspaceID", lease.WorkspaceID, "runID", lease.RunID, "generation", claimedLease.RecoveryGeneration, "err", err)
			return
		}
		gateTransferred = true
	})
	if err != nil {
		return false
	}
	return true
}

func (m *Biz) runtimeDriveLeaseTimerCallbackActive(ctx context.Context, epoch uint64) bool {
	if m == nil || ctx == nil || epoch == 0 || ctx.Err() != nil {
		return false
	}
	m.driveMu.Lock()
	active := !m.runtimeLeaseTimerClosing && m.runtimeLeaseTimerEpoch == epoch
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" && (runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != "") {
		active = false
	}
	m.driveMu.Unlock()
	return active && ctx.Err() == nil
}

// finishRuntimeDriveLeaseTimerAtGateExit replaces the retiring timer's local
// registration with the newest exact scheduled successor in one driveMu
// critical section. The successor can come from an authoritative not-claimed
// response (same stable identity, newer generation) or from the lifecycle
// mount latch (including a different resolution signature).
func (m *Biz) finishRuntimeDriveLeaseTimerAtGateExit(
	ctx context.Context,
	retiring runtimecontract.RuntimeDriveLease,
	authoritativeSuccessor *runtimecontract.RuntimeDriveLease,
	source string,
	epoch uint64,
) {
	key := runtimeDriveKey(retiring.WorkspaceID, retiring.RunID)
	next, retained := m.replaceRuntimeDriveLeaseTimerAtGateExit(key, retiring.RecoveryGeneration, authoritativeSuccessor, epoch)
	if !retained {
		return
	}
	if !m.launchRuntimeDriveLeaseTimer(ctx, next, source+":timer_handoff", epoch) {
		// Background acceptance is only local delivery machinery. Drop the local
		// reservation and gate, but never mutate the persisted scheduled lease.
		m.releaseRejectedRuntimeDriveLeaseTimer(key, next.RecoveryGeneration, epoch)
		slog.Warn("workorch Runtime persisted lease timer handoff was not accepted", "workspaceID", next.WorkspaceID, "runID", next.RunID, "generation", next.RecoveryGeneration, "source", source)
		return
	}
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: next.WorkspaceID, RunID: next.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: next.AggregateRevision, RecoveryGeneration: next.RecoveryGeneration, FailureCategory: string(next.Reason), LeaseState: next.State, Source: source + ":timer_handoff",
	})
}

func (m *Biz) replaceRuntimeDriveLeaseTimerAtGateExit(
	key string,
	retiringGeneration uint64,
	authoritativeSuccessor *runtimecontract.RuntimeDriveLease,
	epoch uint64,
) (runtimecontract.RuntimeDriveLease, bool) {
	if m == nil || key == "" || retiringGeneration == 0 || epoch == 0 {
		return runtimecontract.RuntimeDriveLease{}, false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	registration, registered := m.retryLeaseTimers[key]
	if registered && registration.Generation == retiringGeneration && registration.Epoch == epoch {
		delete(m.retryLeaseTimers, key)
	}
	if m.runtimeLeaseTimerClosing || m.runtimeLeaseTimerEpoch != epoch {
		if gateEpoch, ok := m.drivingRuns[key]; ok && gateEpoch == epoch {
			delete(m.persistedLeaseMounts, key)
			delete(m.drivingRuns, key)
		}
		return runtimecontract.RuntimeDriveLease{}, false
	}
	if workspaceID := runtimeDriveWorkspaceID(key); workspaceID == "" || runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != "" {
		delete(m.persistedLeaseMounts, key)
		delete(m.drivingRuns, key)
		return runtimecontract.RuntimeDriveLease{}, false
	}
	var next runtimecontract.RuntimeDriveLease
	if authoritativeSuccessor != nil && authoritativeSuccessor.Validate() == nil &&
		authoritativeSuccessor.State == runtimecontract.RuntimeDriveLeaseScheduled {
		next = authoritativeSuccessor.Clone()
	}
	if mounted, ok := m.persistedLeaseMounts[key]; ok &&
		(next.WorkspaceID == "" || mounted.RecoveryGeneration > next.RecoveryGeneration) {
		next = mounted.Clone()
	}
	if next.WorkspaceID == "" {
		if gateEpoch, ok := m.drivingRuns[key]; ok && gateEpoch == epoch {
			delete(m.drivingRuns, key)
		}
		return runtimecontract.RuntimeDriveLease{}, false
	}
	if mounted, ok := m.persistedLeaseMounts[key]; ok && mounted.RecoveryGeneration <= next.RecoveryGeneration {
		delete(m.persistedLeaseMounts, key)
	}
	if m.retryLeaseTimers == nil {
		m.retryLeaseTimers = map[string]runtimeLeaseTimerRegistration{}
	}
	m.retryLeaseTimers[key] = runtimeLeaseTimerRegistration{Generation: next.RecoveryGeneration, Epoch: epoch}
	return next, true
}

func (m *Biz) releaseCanceledRuntimeDriveLeaseTimer(key string, generation uint64, epoch uint64) {
	if m == nil || key == "" || generation == 0 || epoch == 0 {
		return
	}
	m.driveMu.Lock()
	if registration, ok := m.retryLeaseTimers[key]; ok && registration.Generation == generation && registration.Epoch == epoch {
		delete(m.retryLeaseTimers, key)
	}
	if gateEpoch, ok := m.drivingRuns[key]; ok && gateEpoch == epoch {
		delete(m.persistedLeaseMounts, key)
		delete(m.drivingRuns, key)
	}
	m.driveMu.Unlock()
}

func (m *Biz) releaseRejectedRuntimeDriveLeaseTimer(key string, generation uint64, epoch uint64) {
	// Rejection has the same local cleanup semantics as Stop cancellation. In
	// both cases persisted RuntimeDriveLease remains the only retry authority.
	m.releaseCanceledRuntimeDriveLeaseTimer(key, generation, epoch)
}

// claimRuntimeDriveLeaseWhileStarted fences the timer's only Runtime owner
// dereference against Stop. The lock is released before handing a claimed
// generation to scheduleRuntimeDriveWithLease, whose worker acquires its own
// lifecycle read lock.
func (m *Biz) claimRuntimeDriveLeaseWhileStarted(ctx context.Context, lease runtimecontract.RuntimeDriveLease, now string, epoch uint64) (runtimecontract.RuntimeDriveLease, bool, error) {
	if m == nil {
		return runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	if !m.runtimeDriveLeaseTimerCallbackActive(ctx, epoch) || m.runtimeController == nil {
		return runtimecontract.RuntimeDriveLease{}, false, context.Canceled
	}
	return m.runtimeClaimDriveLease(ctx, lease, now)
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

// retainRuntimeDriveGateForPersistedLease resolves the only race between an
// external governance-resolution delivery and an already-running SM-4 worker.
// The lease was persisted before this method is called. If a worker still owns
// the gate, its exit path receives an immutable generation hint; otherwise the
// caller becomes the gate owner and may mount the timer itself. This latch is
// neither retry authority nor a work queue: claim still compares the complete
// persisted lease before any drive can run.
func (m *Biz) retainRuntimeDriveGateForPersistedLease(key string, lease runtimecontract.RuntimeDriveLease) bool {
	if m == nil || key == "" || lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return false
	}
	m.runtimeMutationFence.RLock()
	defer m.runtimeMutationFence.RUnlock()
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.runtimeLeaseTimerClosing {
		return false
	}
	if workspaceID := runtimeDriveWorkspaceID(key); workspaceID == "" || runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != "" {
		return false
	}
	if m.runtimeLeaseTimerEpoch == 0 {
		m.runtimeLeaseTimerEpoch = 1
	}
	epoch := m.runtimeLeaseTimerEpoch
	if registration, ok := m.retryLeaseTimers[key]; ok && registration.Generation == lease.RecoveryGeneration && registration.Epoch == epoch {
		return false
	}
	if m.drivingRuns == nil {
		m.drivingRuns = map[string]uint64{}
	}
	if _, running := m.drivingRuns[key]; !running {
		m.drivingRuns[key] = epoch
		return true
	}
	if m.persistedLeaseMounts == nil {
		m.persistedLeaseMounts = map[string]runtimecontract.RuntimeDriveLease{}
	}
	current, exists := m.persistedLeaseMounts[key]
	if !exists || lease.RecoveryGeneration > current.RecoveryGeneration {
		m.persistedLeaseMounts[key] = lease.Clone()
	}
	return false
}

// takePersistedRuntimeDriveLeaseMount is called only by the current gate
// owner. It either retains that gate for the exact persisted lease or releases
// the gate in the same critical section, so a concurrent handoff cannot land
// between the final check and gate release.
func (m *Biz) takePersistedRuntimeDriveLeaseMount(key string) (runtimecontract.RuntimeDriveLease, bool) {
	if m == nil || key == "" {
		return runtimecontract.RuntimeDriveLease{}, false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if lease, ok := m.persistedLeaseMounts[key]; ok {
		delete(m.persistedLeaseMounts, key)
		return lease.Clone(), true
	}
	delete(m.drivingRuns, key)
	return runtimecontract.RuntimeDriveLease{}, false
}

func (m *Biz) clearPersistedRuntimeDriveLeaseMount(key string, generation uint64) {
	if m == nil || key == "" || generation == 0 {
		return
	}
	m.driveMu.Lock()
	if current, ok := m.persistedLeaseMounts[key]; ok && current.RecoveryGeneration <= generation {
		delete(m.persistedLeaseMounts, key)
	}
	m.driveMu.Unlock()
}

func (m *Biz) mountPersistedRuntimeDriveLeaseAtGateExit(ctx context.Context, request scheduledRuntimeDriveRequest) bool {
	lease, retained := m.takePersistedRuntimeDriveLeaseMount(request.driveKey)
	if !retained {
		return false
	}
	if !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, request.source+":gate_handoff") {
		// Timer rejection cannot authorize a direct drive. Release the local
		// gate; the persisted scheduled lease remains available to fresh startup.
		m.endRuntimeDrive(request.driveKey)
		slog.Warn("workorch Runtime persisted lifecycle lease mount failed", "workspaceID", request.workspaceID, "runID", request.runID, "generation", lease.RecoveryGeneration, "source", request.source)
		return false
	}
	m.clearPersistedRuntimeDriveLeaseMount(request.driveKey, lease.RecoveryGeneration)
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: request.source + ":gate_handoff",
	})
	return true
}

func (m *Biz) beginRuntimeDrive(key string) bool {
	if m == nil || key == "" {
		return false
	}
	m.runtimeMutationFence.RLock()
	defer m.runtimeMutationFence.RUnlock()
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.runtimeLeaseTimerClosing {
		return false
	}
	if workspaceID := runtimeDriveWorkspaceID(key); workspaceID == "" || runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != "" {
		return false
	}
	if m.runtimeLeaseTimerEpoch == 0 {
		m.runtimeLeaseTimerEpoch = 1
	}
	if m.drivingRuns == nil {
		m.drivingRuns = map[string]uint64{}
	}
	if _, ok := m.drivingRuns[key]; ok {
		return false
	}
	m.drivingRuns[key] = m.runtimeLeaseTimerEpoch
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

func (m *Biz) beginRuntimeLeaseTimer(key string, generation uint64) (registered bool, accepted bool, epoch uint64) {
	if m == nil || key == "" || generation == 0 {
		return false, false, 0
	}
	m.runtimeMutationFence.RLock()
	defer m.runtimeMutationFence.RUnlock()
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.runtimeLeaseTimerClosing {
		return false, false, m.runtimeLeaseTimerEpoch
	}
	if workspaceID := runtimeDriveWorkspaceID(key); workspaceID == "" || runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != "" {
		return false, false, m.runtimeLeaseTimerEpoch
	}
	if m.runtimeLeaseTimerEpoch == 0 {
		m.runtimeLeaseTimerEpoch = 1
	}
	epoch = m.runtimeLeaseTimerEpoch
	if m.retryLeaseTimers == nil {
		m.retryLeaseTimers = map[string]runtimeLeaseTimerRegistration{}
	}
	if current, ok := m.retryLeaseTimers[key]; ok && current.Generation == generation && current.Epoch == epoch {
		return false, true, epoch
	}
	m.retryLeaseTimers[key] = runtimeLeaseTimerRegistration{Generation: generation, Epoch: epoch}
	return true, true, epoch
}

func (m *Biz) endRuntimeLeaseTimer(key string, generation uint64, epoch uint64) {
	if m == nil || key == "" || generation == 0 || epoch == 0 {
		return
	}
	m.driveMu.Lock()
	if current, ok := m.retryLeaseTimers[key]; ok && current.Generation == generation && current.Epoch == epoch {
		delete(m.retryLeaseTimers, key)
	}
	m.driveMu.Unlock()
}

func (m *Biz) runtimeLifecycleSchedulingError() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.driveMu.Lock()
	closing := m.runtimeLeaseTimerClosing
	ctx := m.lifecycleCtx
	m.driveMu.Unlock()
	if closing {
		return context.Canceled
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func (m *Biz) orchestratorBackgroundContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	bgCtx := context.Background()
	if m != nil {
		m.driveMu.Lock()
		if m.lifecycleCtx != nil {
			bgCtx = m.lifecycleCtx
		}
		m.driveMu.Unlock()
	}
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
	if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return orchestratorevents.OrchestratorCommandResult{}, err
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
	var executionBudgetExhausted *runtimecontract.RuntimeBudgetExhaustedError
	var timeout interface{ Timeout() bool }
	var temporary interface{ Temporary() bool }
	switch {
	case errors.As(err, &executionBudgetExhausted):
		return runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionBudgetExhausted, Code: "execution_budget_exhausted",
			Signature: "execution-budget-exhausted/" + string(executionBudgetExhausted.Dimension), Message: msg,
		}
	case errors.As(err, &capabilityCatalogUnavailable):
		signature := "runtime-capability-catalog-unavailable"
		if owner := strings.TrimSpace(capabilityCatalogUnavailable.OwnerModuleID); owner != "" {
			signature += "/" + owner
		}
		if subject := strings.TrimSpace(capabilityCatalogUnavailable.SubjectRef); subject != "" {
			signature += "/" + subject
		}
		return runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassCapabilityUnavailable, Code: "runtime_capability_catalog_unavailable",
			Signature: signature, Message: msg,
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

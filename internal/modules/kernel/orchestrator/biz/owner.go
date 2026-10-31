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
	subscriptions      []ownerSubscription
	ownerLifecycle     basebiz.Lifecycle
	shutdownMu         sync.Mutex
	releasing          bool
	released           bool
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
	drivingRuns           map[string]runtimeDriveGateRegistration
	runtimeDriveGateToken uint64
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
	retryLeaseTimers map[string]runtimeLeaseTimerRegistration
	// runtimeDriveGateSettlements is a lifecycle-owned wakeup dedupe for an
	// owner exit whose durable DriveLease could not yet be authoritatively
	// loaded, transitioned, or mounted. The entry is never Runtime authority:
	// every wakeup fresh-loads the Run aggregate and may mutate it only through
	// the existing Orchestrator-private lease events. Epoch/token fencing keeps
	// an old callback from clearing a newer timer or gate.
	runtimeDriveGateSettlements       map[string]runtimeDriveGateSettlementRegistration
	runtimeDriveGateSettlementToken   uint64
	runtimeDriveGateSettlementRunning bool
	runtimeDriveGateSettlementWake    runtimeDriveGateSettlementWakeRegistration
	runtimeLeaseTimerCtx              context.Context
	runtimeLeaseTimerCancel           context.CancelFunc
	lifecycleCtx                      context.Context
	lifecycleCancel                   context.CancelFunc
	runtimeLeaseTimerEpoch            uint64
	runtimeLeaseTimerClosing          bool
	started                           bool
}

type runtimeLeaseTimerRegistration struct {
	Generation uint64
	Epoch      uint64
	GateToken  uint64
	Lease      runtimecontract.RuntimeDriveLease
}

type runtimeDriveGateRegistration struct {
	Epoch uint64
	Token uint64
}

type runtimeDriveGateSettlementRegistration struct {
	Epoch     uint64
	GateToken uint64
	Token     uint64
	Request   runtimeDriveGateSettlementRequest
}

type runtimeDriveGateSettlementWakeRegistration struct {
	Epoch     uint64
	Token     uint64
	Timer     *time.Timer
	Submitted bool
}

type runtimeStartupRecoveryActionKind string

const (
	runtimeStartupRecoveryActionDrive                    runtimeStartupRecoveryActionKind = "drive"
	runtimeStartupRecoveryActionRecoverLease             runtimeStartupRecoveryActionKind = "recover_lease"
	runtimeStartupRecoveryActionMaterializeTerminalLease runtimeStartupRecoveryActionKind = "materialize_terminal_lease"
)

type runtimeStartupRecoveryAction struct {
	Kind   runtimeStartupRecoveryActionKind
	Item   runevents.RuntimeRecoveryInventoryItem
	Source string
}

type runtimeDriveGateSettlementTask struct {
	owner    *Biz
	epoch    uint64
	token    uint64
	callback func()
}

func (t *runtimeDriveGateSettlementTask) Run() {
	if t != nil && t.callback != nil {
		t.callback()
		return
	}
	if t != nil && t.owner != nil {
		t.owner.runRuntimeDriveGateSettlementPumpCycle(t.epoch, t.token)
	}
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
	leave, err := m.enterOwner(false)
	if err != nil {
		return err
	}
	defer leave()
	if m.started {
		return nil
	}
	m.shutdownMu.Lock()
	if m.runtimeLeaseTimerClosing {
		m.shutdownMu.Unlock()
		return basebiz.ErrLifecycleClosing
	}
	err = m.openRuntimeLifecycle(ctx)
	m.shutdownMu.Unlock()
	if err != nil {
		return err
	}
	startupSucceeded := false
	defer func() {
		if !startupSucceeded {
			m.BeginShutdown(context.Background())
		}
	}()
	lifecycleCtx := m.runtimeLifecycleContext()
	// Initialize the SM-4 exit-settlement wake lifecycle before any startup
	// recovery action can acquire a run gate. A one-shot owner timer exists only
	// while a settlement is pending and submits a short BackgroundRoutine task;
	// rejection re-arms the exact epoch/token without occupying a worker.
	if err := m.startRuntimeDriveGateSettlementPump(lifecycleCtx); err != nil {
		return fmt.Errorf("Runtime drive gate settlement pump: %w", err)
	}
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
	if err := m.subscribeRuntimeIngress(); err != nil {
		return err
	}
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

func (m *Biz) subscribeRuntimeIngress() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if len(m.subscriptions) != 0 {
		return common.ErrConflict
	}
	topics := []string{
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
	for _, topic := range topics {
		switch topic {
		case orchestratorevents.TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence,
			orchestratorevents.TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence:
			if err := m.subscribeOwner(topic, m.handleWorkspaceRuntimeQuiescence, false); err != nil {
				return err
			}
		case orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion,
			orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence:
			if err := m.subscribeOwner(topic, m.handleWorkspaceDeletion, false); err != nil {
				return err
			}
		default:
			if err := m.subscribeOwner(topic, m.handleMagicCommand, false); err != nil {
				return err
			}
		}
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorDriveRuntime, m.handleRuntimeDrive, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, m.handleRuntimeOperationReceipt, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput, m.handleRuntimeValidationOutput, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, m.handleRuntimeOperationReceipt, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, m.handleRuntimeGovernanceTicketResolution, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval, m.handleRuntimeGovernanceApprovalResolution, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, m.handleRuntimeTerminationRequest, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence, m.handleDecisionEvidenceCommand, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence, m.handleApprovalEvidenceCommand, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(runevents.TopicRunStateChanged, m.handleRunStateChanged, true); err != nil {
		return err
	}
	if err := m.subscribeOwner(runevents.TopicRunContinuationRequested, m.handleRunContinuationRequested, true); err != nil {
		return err
	}
	if err := m.subscribeOwner(runevents.TopicRunRuntimeCommandsReady, m.handleRunRuntimeCommandsReady, true); err != nil {
		return err
	}
	return nil
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
	key := runtimeDriveKey(cmd.WorkspaceID, cmd.RunID)
	gateToken, acquiredGate := m.beginRuntimeDriveWithToken(key)
	gate, ownsGate := m.currentRuntimeDriveGate(key)
	if !ownsGate || gate.Token == 0 || (acquiredGate && gate.Token != gateToken) {
		setOrchestratorAck(result, fmt.Errorf("%w: Runtime termination drive gate is unavailable", common.ErrConflict))
		return
	}
	settlementCtx := m.orchestratorBackgroundContext(ev.Context(), cmd.WorkspaceID)
	terminated, err := m.submitRuntimeInternalEvent(ev.Context(), runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventTerminationRequested, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{TerminationKind: cmd.Kind, RequestedBy: cmd.RequestedBy, Reason: cmd.Reason},
	})
	if err != nil {
		// The termination CAS may have committed while its response was lost.
		// Continue only from a fresh aggregate carrying this exact Run-owned
		// intent; a different or absent record proves that this command did not
		// obtain termination authority.
		fresh, loadErr := m.runtimeController.Load(settlementCtx, cmd.WorkspaceID, cmd.RunID)
		if loadErr != nil {
			// The owner CAS outcome is unknown. Keep the exact gate and let the
			// lifecycle-fenced settlement wake fresh-load it again; releasing here
			// could orphan either the newly persisted terminal or the still-active
			// pre-termination aggregate.
			m.retainOrAcquireRuntimeDriveGateSettlement(runtimeDriveGateSettlementRequest{
				WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, DriveKey: key,
				GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "administrative_termination_commit_unknown",
				ReleaseWhenNoLease: false, AwaitGateOwnerExit: !acquiredGate,
			})
			setOrchestratorAck(result, err)
			return
		}
		if !runtimeTerminationCommandMatchesAggregate(cmd, fresh) {
			if acquiredGate {
				m.settleRuntimeDriveGateAtOwnerExit(settlementCtx, runtimeDriveGateSettlementRequest{
					WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, DriveKey: key,
					GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "administrative_termination_not_committed",
					ReleaseWhenNoLease: true,
				})
			}
			setOrchestratorAck(result, err)
			return
		}
		terminated.Aggregate = fresh
	}
	aggregate := terminated.Aggregate
	request, err := terminalDeliveryRequest(aggregate)
	if err != nil {
		m.retainOrAcquireRuntimeDriveGateSettlement(runtimeDriveGateSettlementRequest{
			WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, DriveKey: key,
			GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "administrative_termination_delivery_contract_unknown",
			ReleaseWhenNoLease: false, AwaitGateOwnerExit: !acquiredGate,
		})
		setOrchestratorAck(result, err)
		return
	}
	if !acquiredGate {
		// An already-running drive owns the only execution lane. The termination
		// CAS is durable and supersedes its stale work; register only an exact
		// lifecycle wakeup so that worker (or the pump after it exits) performs
		// terminal delivery. Never deliver concurrently beside that gate.
		retained := m.retainRuntimeDriveGateSettlement(runtimeDriveGateSettlementRequest{
			WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, DriveKey: key,
			GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "administrative_termination_handoff", ReleaseWhenNoLease: false,
			AwaitGateOwnerExit: true,
		})
		if !retained {
			// The old owner can leave between the termination CAS and local handoff.
			// Reacquire the now-free gate and perform first delivery here; if another
			// owner won that race, attach the dormant handoff to its exact token.
			gateToken, acquiredGate = m.beginRuntimeDriveWithToken(key)
			gate, ownsGate = m.currentRuntimeDriveGate(key)
			if !ownsGate || gate.Token == 0 || (acquiredGate && gate.Token != gateToken) {
				setOrchestratorAck(result, fmt.Errorf("%w: Runtime termination drive gate is unavailable", common.ErrConflict))
				return
			}
			if !acquiredGate {
				retained = m.retainRuntimeDriveGateSettlement(runtimeDriveGateSettlementRequest{
					WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, DriveKey: key,
					GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "administrative_termination_handoff_reacquired", ReleaseWhenNoLease: false,
					AwaitGateOwnerExit: true,
				})
			}
		}
		if acquiredGate {
			// This handler is now the exact gate owner and falls through to the same
			// first-delivery/failure settlement as an initially free run.
		} else if retained {
			current, loadErr := m.runtimeController.Load(settlementCtx, cmd.WorkspaceID, cmd.RunID)
			if loadErr == nil && deliveryAcknowledged(current, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerWorkspace) {
				m.setRuntimeTerminationResult(result, request.TerminalDeliveryID, current)
				return
			}
			setOrchestratorAck(result, fmt.Errorf("%w: Runtime termination delivery is owned by the active drive", common.ErrConflict))
			return
		} else {
			setOrchestratorAck(result, fmt.Errorf("%w: Runtime termination drive gate handoff failed", common.ErrConflict))
			return
		}
	}
	err = m.deliverRuntimeTerminal(ev.Context(), aggregate)
	if err != nil {
		failedGeneration := uint64(0)
		if retired := aggregate.Facts.Recovery.Normalized().RetiredTerminalDeliveryLease; retired.Validate() == nil &&
			retired.State == runtimecontract.RuntimeDriveLeaseRunning {
			failedGeneration = retired.RecoveryGeneration
		}
		driveRequest := scheduledRuntimeDriveRequest{
			workspaceID: cmd.WorkspaceID, runID: cmd.RunID, source: "administrative_termination_delivery",
			driveKey: key, leaseGeneration: failedGeneration, gateEpoch: gate.Epoch, gateToken: gate.Token,
		}
		retry := m.handleRuntimeTerminalDeliveryRetry(ev.Context(), driveRequest, runtimeTerminalDeliveryFailure(err))
		m.finishScheduledRuntimeDrive(settlementCtx, driveRequest, &retry)
		current, loadErr := m.runtimeController.Load(settlementCtx, cmd.WorkspaceID, cmd.RunID)
		if loadErr == nil && deliveryAcknowledged(current, request.TerminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerWorkspace) {
			m.setRuntimeTerminationResult(result, request.TerminalDeliveryID, current)
			return
		}
		setOrchestratorAck(result, err)
		return
	}
	completed, err := m.runtimeController.Load(settlementCtx, cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		m.settleRuntimeDriveGateAtOwnerExit(settlementCtx, runtimeDriveGateSettlementRequest{
			WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, DriveKey: key,
			GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "administrative_termination_response_reload", ReleaseWhenNoLease: true,
		})
		setOrchestratorAck(result, err)
		return
	}
	m.settleRuntimeDriveGateAtOwnerExit(settlementCtx, runtimeDriveGateSettlementRequest{
		WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, DriveKey: key,
		GateEpoch: gate.Epoch, GateToken: gate.Token, Source: "administrative_termination_complete", ReleaseWhenNoLease: true,
	})
	m.setRuntimeTerminationResult(result, request.TerminalDeliveryID, completed)
}

func runtimeTerminationCommandMatchesAggregate(cmd orchestratorevents.RequestRuntimeTerminationCommand, aggregate runtime.RuntimeAggregate) bool {
	if aggregate.Termination == nil || aggregate.Facts.WorkspaceID != cmd.WorkspaceID || aggregate.Facts.RunID != cmd.RunID {
		return false
	}
	termination := aggregate.Termination.Normalized()
	return termination.Validate() == nil && termination.Kind == cmd.Kind.Normalized() &&
		termination.RequestedBy == strings.TrimSpace(cmd.RequestedBy) && termination.Reason == strings.TrimSpace(cmd.Reason)
}

func (m *Biz) setRuntimeTerminationResult(result magicevent.Result, terminalDeliveryID string, aggregate runtime.RuntimeAggregate) {
	if result == nil {
		return
	}
	result.Set(orchestratorevents.RequestRuntimeTerminationResult{
		TerminalDeliveryID: terminalDeliveryID,
		WorkspaceReleased:  deliveryAcknowledged(aggregate, terminalDeliveryID, runtimecontract.RuntimeDeliveryOwnerWorkspace),
		StateRevision:      aggregate.StateRevision,
	}, nil)
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
			if aggregate.Facts.Recovery.Normalized().TerminalDeliveryIntervention.Validate() == nil {
				continue
			}
			if !runtimeTerminalDeliveryFullyAcknowledged(aggregate) {
				kind := runtimeStartupRecoveryActionDrive
				recovery := aggregate.Facts.Recovery.Normalized()
				if recovery.TerminalDeliveryRequest.Validate() == nil ||
					(recovery.RetiredTerminalDeliveryLease.Validate() == nil && recovery.RetiredTerminalDeliveryLease.State == runtimecontract.RuntimeDriveLeaseRunning) {
					kind = runtimeStartupRecoveryActionMaterializeTerminalLease
				}
				actions = append(actions, runtimeStartupRecoveryAction{Kind: kind, Item: item, Source: "startup_recovery:workspace_retirement_terminal_delivery"})
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
		case aggregate.Facts.Recovery.Normalized().TerminalDeliveryIntervention.Validate() == nil:
			continue
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
		kind := runtimeStartupRecoveryActionDrive
		if runtimeTerminalDeliveryReady(aggregate) && !runtimeTerminalDeliveryFullyAcknowledged(aggregate) {
			recovery := aggregate.Facts.Recovery.Normalized()
			if recovery.TerminalDeliveryRequest.Validate() == nil ||
				(recovery.RetiredTerminalDeliveryLease.Validate() == nil && recovery.RetiredTerminalDeliveryLease.State == runtimecontract.RuntimeDriveLeaseRunning) {
				kind = runtimeStartupRecoveryActionMaterializeTerminalLease
			}
		}
		actions = append(actions, runtimeStartupRecoveryAction{Kind: kind, Item: item, Source: source})
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
	var failures []error
	for _, action := range actions {
		if err := m.releaseRuntimeStartupRecoveryAction(ctx, action); err != nil {
			if m.retainRuntimeStartupRecoveryActionFailure(ctx, action) {
				slog.Warn("workorch Runtime startup recovery action retained for lifecycle settlement", "workspaceID", action.Item.WorkspaceID, "runID", action.Item.RunID, "source", action.Source, "err", err)
				continue
			}
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (m *Biz) releaseRuntimeStartupRecoveryAction(ctx context.Context, action runtimeStartupRecoveryAction) error {
	item := action.Item
	switch action.Kind {
	case runtimeStartupRecoveryActionDrive:
		if err := m.scheduleRuntimeDrive(ctx, item.WorkspaceID, item.RunID, action.Source); err != nil {
			if runtimeDriveScheduleSettlementWasRetained(err) {
				return nil
			}
			return fmt.Errorf("schedule workspace=%s run=%s source=%s: %w", item.WorkspaceID, item.RunID, action.Source, err)
		}
	case runtimeStartupRecoveryActionRecoverLease:
		aggregate, err := m.runtimeController.Load(ctx, item.WorkspaceID, item.RunID)
		if err != nil {
			return fmt.Errorf("reload lease workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, err)
		}
		key := runtimeDriveKey(item.WorkspaceID, item.RunID)
		if !m.beginRuntimeDrive(key) {
			return nil
		}
		settlement := runtimeDriveGateSettlementRequest{WorkspaceID: item.WorkspaceID, RunID: item.RunID, DriveKey: key, Source: action.Source}
		if aggregate.DriveLease != nil {
			settlement.LeaseHint, settlement.HasLeaseHint = aggregate.DriveLease.Clone(), true
		} else {
			settlement.Source += ":lease_reloaded"
		}
		if !m.settleRuntimeDriveGateOrRetain(ctx, settlement) {
			if epoch, token := m.runtimeDriveGateSettlementIdentity(key); epoch == 0 || token == 0 {
				return fmt.Errorf("reconcile lease workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, common.ErrConflict)
			}
		}
	case runtimeStartupRecoveryActionMaterializeTerminalLease:
		aggregate, err := m.runtimeController.Load(ctx, item.WorkspaceID, item.RunID)
		if err != nil {
			return fmt.Errorf("reload terminal recovery workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, err)
		}
		recovery := aggregate.Facts.Recovery.Normalized()
		authorization := recovery.TerminalDeliveryRequest
		retired := recovery.RetiredTerminalDeliveryLease
		if !runtimeTerminalDeliveryReady(aggregate) || runtimeTerminalDeliveryFullyAcknowledged(aggregate) ||
			(authorization.Validate() != nil && (retired.Validate() != nil || retired.State != runtimecontract.RuntimeDriveLeaseRunning)) {
			return fmt.Errorf("%w: terminal recovery authorization workspace=%s run=%s is not actionable", common.ErrConflict, item.WorkspaceID, item.RunID)
		}
		key := runtimeDriveKey(item.WorkspaceID, item.RunID)
		if !m.beginRuntimeDrive(key) {
			return nil
		}
		settlement := runtimeDriveGateSettlementRequest{
			WorkspaceID: item.WorkspaceID, RunID: item.RunID, DriveKey: key, Source: action.Source,
		}
		if authorization.Validate() == nil {
			retry := runtimeDeliveryRecoveryLease(action.Source, "terminal-delivery", authorization.RequestID)
			retry.FromRunningGeneration = authorization.FailedLeaseGeneration
			settlement.Retry, settlement.HasRetry = *retry, true
		}
		if !m.settleRuntimeDriveGateOrRetain(ctx, settlement) {
			if epoch, token := m.runtimeDriveGateSettlementIdentity(key); epoch == 0 || token == 0 {
				return fmt.Errorf("materialize terminal lease workspace=%s run=%s: %w", item.WorkspaceID, item.RunID, common.ErrConflict)
			}
		}
	default:
		return common.ErrInvalidArgument
	}
	return nil
}

func (m *Biz) retainRuntimeStartupRecoveryActionFailure(_ context.Context, action runtimeStartupRecoveryAction) bool {
	item := action.Item
	if item.WorkspaceID == "" || item.RunID == "" {
		return false
	}
	key := runtimeDriveKey(item.WorkspaceID, item.RunID)
	if _, ownsGate := m.currentRuntimeDriveGate(key); !ownsGate {
		if !m.beginRuntimeDrive(key) {
			// A durable retirement/quiescence fence makes this action intentionally
			// inert; another live gate already owns every other rejection case.
			_, ownsGate = m.currentRuntimeDriveGate(key)
			return ownsGate || m.workspaceRuntimeDriveAdmissionClosed(item.WorkspaceID)
		}
	}
	gate, ownsGate := m.currentRuntimeDriveGate(key)
	if !ownsGate {
		return false
	}
	settlement := runtimeDriveGateSettlementRequest{
		WorkspaceID: item.WorkspaceID, RunID: item.RunID, DriveKey: key,
		GateEpoch: gate.Epoch, GateToken: gate.Token,
		Source: action.Source + ":startup_action_failure",
	}
	return m.retainRuntimeDriveGateSettlement(settlement)
}

func (m *Biz) workspaceRuntimeDriveAdmissionClosed(workspaceID common.WorkspaceID) bool {
	if m == nil || workspaceID == "" {
		return false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	return runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != ""
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

func (m *Biz) startRuntimeDriveGateSettlementPump(ctx context.Context) error {
	if m == nil || ctx == nil || m.BackgroundRoutine() == nil {
		return common.ErrInvalidArgument
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.runtimeLeaseTimerClosing || m.lifecycleCtx == nil || m.lifecycleCtx != ctx || ctx.Err() != nil || m.runtimeDriveGateSettlementWake.Epoch != 0 {
		return fmt.Errorf("%w: Runtime drive gate settlement pump lifecycle is unavailable", common.ErrConflict)
	}
	return nil
}

const runtimeDriveGateSettlementWakeDelay = time.Second

// armRuntimeDriveGateSettlementWakeLocked registers one lifecycle-fenced
// wakeup only while an unresolved gate settlement exists. The timer itself is
// process-local scheduling machinery; the short BackgroundRoutine task is the
// only code allowed to fresh-load or mutate Runtime authority.
func (m *Biz) armRuntimeDriveGateSettlementWakeLocked(delay time.Duration) {
	if m == nil || m.runtimeLeaseTimerClosing || m.lifecycleCtx == nil || m.lifecycleCtx.Err() != nil ||
		!m.runtimeDriveGateSettlementWakeNeededLocked() || m.runtimeDriveGateSettlementWake.Epoch != 0 {
		return
	}
	if delay <= 0 {
		delay = runtimeDriveGateSettlementWakeDelay
	}
	m.runtimeDriveGateSettlementToken++
	if m.runtimeDriveGateSettlementToken == 0 {
		m.runtimeDriveGateSettlementToken++
	}
	epoch := m.runtimeLeaseTimerEpoch
	token := m.runtimeDriveGateSettlementToken
	wake := runtimeDriveGateSettlementWakeRegistration{Epoch: epoch, Token: token}
	wake.Timer = time.AfterFunc(delay, func() { m.submitRuntimeDriveGateSettlementWake(epoch, token) })
	m.runtimeDriveGateSettlementWake = wake
}

func (m *Biz) runtimeDriveGateSettlementWakeNeededLocked() bool {
	for _, registration := range m.runtimeDriveGateSettlements {
		if !registration.Request.AwaitGateOwnerExit {
			return true
		}
	}
	return false
}

func (m *Biz) submitRuntimeDriveGateSettlementWake(epoch, token uint64) {
	if m == nil || epoch == 0 || token == 0 {
		return
	}
	m.driveMu.Lock()
	wake := m.runtimeDriveGateSettlementWake
	if m.runtimeLeaseTimerClosing || m.runtimeLeaseTimerEpoch != epoch || wake.Epoch != epoch || wake.Token != token || wake.Submitted {
		m.driveMu.Unlock()
		return
	}
	if !m.runtimeDriveGateSettlementWakeNeededLocked() {
		m.runtimeDriveGateSettlementWake = runtimeDriveGateSettlementWakeRegistration{}
		m.driveMu.Unlock()
		return
	}
	wake.Timer = nil
	wake.Submitted = true
	m.runtimeDriveGateSettlementWake = wake
	m.driveMu.Unlock()
	background := m.BackgroundRoutine()
	if background != nil {
		if err := m.scheduleOwnerWork(func(callback func()) error {
			return background.AsyncTask(&runtimeDriveGateSettlementTask{callback: callback})
		}, func() { m.runRuntimeDriveGateSettlementPumpCycle(epoch, token) }); err == nil {
			return
		} else {
			slog.Warn("workorch Runtime drive gate settlement wake rejected", "err", err)
		}
	}
	m.driveMu.Lock()
	if current := m.runtimeDriveGateSettlementWake; current.Epoch == epoch && current.Token == token && current.Submitted {
		m.runtimeDriveGateSettlementWake = runtimeDriveGateSettlementWakeRegistration{}
		m.armRuntimeDriveGateSettlementWakeLocked(runtimeDriveGateSettlementWakeDelay)
	}
	m.driveMu.Unlock()
}

func (m *Biz) runRuntimeDriveGateSettlementPumpCycle(epoch, token uint64) {
	if m == nil || epoch == 0 || token == 0 {
		return
	}
	m.driveMu.Lock()
	wake := m.runtimeDriveGateSettlementWake
	active := !m.runtimeLeaseTimerClosing && m.runtimeLeaseTimerEpoch == epoch && wake.Epoch == epoch && wake.Token == token && wake.Submitted
	m.driveMu.Unlock()
	if !active {
		return
	}
	defer func() {
		if value := recover(); value != nil {
			slog.Error("workorch Runtime drive gate settlement pump recovered panic", "panic", value)
		}
		m.driveMu.Lock()
		if current := m.runtimeDriveGateSettlementWake; current.Epoch == epoch && current.Token == token {
			m.runtimeDriveGateSettlementWake = runtimeDriveGateSettlementWakeRegistration{}
			m.armRuntimeDriveGateSettlementWakeLocked(runtimeDriveGateSettlementWakeDelay)
		}
		m.driveMu.Unlock()
	}()
	m.reconcilePendingRuntimeDriveGateSettlements()
}

func (m *Biz) rollbackRuntimeStart() {
	m.BeginShutdown(context.Background())
}

func (m *Biz) unsubscribeRuntimeIngress(ctx context.Context) error {
	for len(m.subscriptions) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.subscriptions) - 1
		entry := m.subscriptions[last]
		var err error
		if entry.matched {
			err = m.UnsubscribeMatchedFuncChecked(ModuleID+"/run-drive", entry.topic)
		} else {
			err = m.UnsubscribeFuncChecked(entry.topic)
		}
		if err != nil {
			return err
		}
		m.subscriptions = m.subscriptions[:last]
	}
	return nil
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
	m.BeginShutdown(ctx)
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	if m.releasing {
		m.shutdownMu.Unlock()
		return common.ErrConflict
	}
	m.releasing = true
	m.shutdownMu.Unlock()
	defer func() { m.shutdownMu.Lock(); m.releasing = false; m.shutdownMu.Unlock() }()
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	if err := m.lockLifecycleForStop(ctx); err != nil {
		return err
	}
	defer m.lifecycleMu.Unlock()
	if err := m.unsubscribeRuntimeIngress(ctx); err != nil {
		return err
	}
	m.started = false
	m.runtimeController.ReleaseHistoricalCompilation()
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
	m.runtimeDriveGateSettlements = nil
	m.runtimeDriveGateSettlementRunning = false
	m.runtimeDriveGateSettlementWake = runtimeDriveGateSettlementWakeRegistration{}
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
	m.shutdownMu.Lock()
	m.released = true
	m.shutdownMu.Unlock()
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
	// The Run fact is already committed. Its request may have ended before
	// this asynchronous callback runs; admission belongs to our lifecycle,
	// just like the scheduled worker. Stop and retirement fences still apply.
	ctx := m.orchestratorBackgroundContext(ev.Context(), data.WorkspaceID)
	var scheduleErr error
	if requestID := strings.TrimSpace(data.ReconciliationRequestID); requestID != "" {
		scheduleErr = m.scheduleRuntimeDriveWithReconciliation(ctx, data.WorkspaceID, data.RunID, "run_state_active_resume", requestID, 0)
	} else {
		scheduleErr = m.scheduleRuntimeDrive(ctx, data.WorkspaceID, data.RunID, "run_state_active")
	}
	if err := scheduleErr; err != nil && !errors.Is(err, context.Canceled) && !runtimeDriveScheduleSettlementWasRetained(err) {
		slog.Warn("workorch Runtime drive scheduling rejected", "source", "run_state_active", "workspaceID", data.WorkspaceID, "runID", data.RunID, "err", err)
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
	ctx := m.orchestratorBackgroundContext(ev.Context(), data.WorkspaceID)
	if err := m.scheduleRuntimeDriveWithReconciliation(ctx, data.WorkspaceID, data.RunID, "run_continuation_requested", data.RequestID, 0); err != nil && !errors.Is(err, context.Canceled) && !runtimeDriveScheduleSettlementWasRetained(err) {
		slog.Warn("workorch Runtime continuation drive scheduling rejected", "workspaceID", data.WorkspaceID, "runID", data.RunID, "err", err)
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
	return m.scheduleRuntimeDriveWithLeaseAndReconciliation(ctx, workspaceID, runID, source, 0, "", 0, 0, 0)
}

type runtimeDriveScheduleSettlementRetainedError struct{ cause error }

func (e *runtimeDriveScheduleSettlementRetainedError) Error() string {
	if e == nil || e.cause == nil {
		return "Runtime drive scheduling was retained for lifecycle settlement"
	}
	return e.cause.Error()
}

func (e *runtimeDriveScheduleSettlementRetainedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func runtimeDriveScheduleSettlementWasRetained(err error) bool {
	var retained *runtimeDriveScheduleSettlementRetainedError
	return errors.As(err, &retained)
}

// scheduleRuntimeDriveWithReconciliation is used by the Run-owner
// continuation fact.  The request identity is carried only until the
// Orchestrator persists the next reconciliation generation; it is never a
// target state or a capability-owner decision.
func (m *Biz) scheduleRuntimeDriveWithReconciliation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source, reconciliationRequestID string, reconciliationGeneration uint64) error {
	return m.scheduleRuntimeDriveWithLeaseAndReconciliation(ctx, workspaceID, runID, source, 0, reconciliationRequestID, reconciliationGeneration, 0, 0)
}

// scheduleRuntimeDriveWithLease runs an already-claimed persisted lease
// without releasing the process-local gate in the handoff between timer and
// drive. A normal Start/Resume has generation zero and first acquires the gate.
func (m *Biz) scheduleRuntimeDriveWithLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string, leaseGeneration, gateEpoch, gateToken uint64) error {
	return m.scheduleRuntimeDriveWithLeaseAndReconciliation(ctx, workspaceID, runID, source, leaseGeneration, "", 0, gateEpoch, gateToken)
}

func (m *Biz) scheduleRuntimeDriveWithLeaseAndReconciliation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string, leaseGeneration uint64, reconciliationRequestID string, reconciliationGeneration, expectedGateEpoch, expectedGateToken uint64) error {
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
	var gateToken uint64
	if leaseGeneration == 0 {
		var accepted bool
		gateToken, accepted = m.beginRuntimeDriveWithToken(driveKey)
		if !accepted {
			return nil
		}
	}
	gate, ownsGate := m.currentRuntimeDriveGate(driveKey)
	if !ownsGate || gate.Token == 0 || (gateToken != 0 && gate.Token != gateToken) ||
		(expectedGateEpoch != 0 && gate.Epoch != expectedGateEpoch) ||
		(expectedGateToken != 0 && gate.Token != expectedGateToken) {
		return common.ErrConflict
	}
	gateToken = gate.Token
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: workspaceID, RunID: runID, Kind: orchestratorevents.RuntimeDriveObservationScheduled,
		RecoveryGeneration: leaseGeneration, Source: source,
	})
	request := scheduledRuntimeDriveRequest{
		workspaceID: workspaceID, runID: runID, source: source, driveKey: driveKey, leaseGeneration: leaseGeneration,
		gateEpoch: gate.Epoch, gateToken: gate.Token,
		reconciliationRequestID: reconciliationRequestID, reconciliationGeneration: reconciliationGeneration,
	}
	bgCtx := m.orchestratorBackgroundContext(ctx, workspaceID)
	// A timer already runs inside BackgroundRoutine. Execute the claimed lease
	// there so the persisted running lease and local gate have no handoff gap.
	if m.BackgroundRoutine() == nil || leaseGeneration != 0 {
		m.runScheduledRuntimeDrive(bgCtx, request)
		return nil
	}
	if err := m.scheduleOwnerWork(m.BackgroundRoutine().AsyncFunction, func() { m.runScheduledRuntimeDrive(bgCtx, request) }); err != nil {
		gate, ownsGate := m.currentRuntimeDriveGate(driveKey)
		if ownsGate && gate.Epoch == request.gateEpoch && gate.Token == gateToken {
			// Submission commonly happens on a Run-owner event lane. Retain the
			// exact gate only; the Start-installed lifecycle pump performs the
			// fresh-load/CAS settlement outside that lane.
			if m.retainRuntimeDriveGateSettlement(runtimeDriveGateSettlementRequest{
				WorkspaceID: workspaceID, RunID: runID, DriveKey: driveKey,
				GateEpoch: gate.Epoch, GateToken: gate.Token, Source: source + ":drive_submission_rejected", ReleaseWhenNoLease: false,
			}) {
				return &runtimeDriveScheduleSettlementRetainedError{cause: err}
			}
		}
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
	gateEpoch                uint64
	gateToken                uint64
	reconciliationRequestID  string
	reconciliationGeneration uint64
}

func (m *Biz) runScheduledRuntimeDrive(ctx context.Context, request scheduledRuntimeDriveRequest) {
	var retry *runtimeRecoveryLeaseRequest
	m.lifecycleMu.RLock()
	if !m.runtimeScheduledDriveRequestActive(request) {
		m.lifecycleMu.RUnlock()
		return
	}
	if m.runtimeController == nil || m.EventHub() == nil {
		m.lifecycleMu.RUnlock()
		m.endRuntimeDriveExact(request.driveKey, request.gateEpoch, request.gateToken)
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
			var handled bool
			retry, handled = m.handleScheduledRuntimeDriveFailure(ctx, request, panicFailure)
			if !handled {
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
			var failureHandled bool
			retry, failureHandled = m.handleScheduledRuntimeDriveFailure(ctx, request, err)
			if !failureHandled {
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
	retry, _ = m.handleScheduledRuntimeDriveFailure(ctx, request, err)
}

func (m *Biz) finishScheduledRuntimeDrive(ctx context.Context, request scheduledRuntimeDriveRequest, retry **runtimeRecoveryLeaseRequest) {
	retryPending := *retry != nil
	if *retry != nil && (*retry).AwaitingNonTerminalRecovery {
		// The canonical nonterminal event owns budget+lease atomically. Whether
		// its response was received or lost, finish may only fresh-reconcile that
		// exact request and mount its persisted lease; it must never fall through
		// to the legacy two-step DriveLeaseScheduled path.
		settlement := runtimeDriveGateSettlementForScheduledRequest(request, *retry, false)
		if !m.settleRuntimeDriveGateAtOwnerExit(ctx, settlement) {
			slog.Warn("workorch Runtime retaining drive gate while nonterminal recovery settlement is unresolved", "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "source", request.source)
		}
		return
	}
	if *retry != nil && (*retry).AwaitingTerminalDeliveryBudget {
		// The budget CAS has not yet earned a durable successor. Reconcile only
		// from fresh owner state now and through the lifecycle pump; never release
		// the gate or manufacture an unbudgeted lease from this local marker.
		settlement := runtimeDriveGateSettlementForScheduledRequest(request, *retry, false)
		if !m.settleRuntimeDriveGateAtOwnerExit(ctx, settlement) {
			slog.Warn("workorch Runtime retaining drive gate while terminal delivery budget settlement is unresolved", "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "source", request.source)
		}
		return
	}
	if *retry != nil && request.leaseGeneration != 0 {
		// Only the worker that claimed this exact generation may replace its
		// running lease. Carry the generation into the owner-private scheduling
		// event so repeated delivery failure cannot collapse into a running
		// duplicate that is then released without a successor.
		(*retry).FromRunningGeneration = request.leaseGeneration
	}
	if *retry != nil && !runtimeRetryUsesExistingPersistedLease(**retry) {
		switch m.scheduleRuntimeRecoveryLeaseDisposition(ctx, request.workspaceID, request.runID, **retry) {
		case runtimeRecoveryLeaseScheduleMounted:
			return
		case runtimeRecoveryLeaseScheduleTerminalComplete:
			// The final owner acknowledgement won after error classification but
			// before the lease CAS. No durable successor is required, so this old
			// worker must release the process gate instead of treating the no-op as
			// a persistence failure.
			m.settleRuntimeDriveGateAtOwnerExit(ctx, runtimeDriveGateSettlementForScheduledRequest(request, nil, true))
			return
		}
	}
	if retryPending {
		settlement := runtimeDriveGateSettlementForScheduledRequest(request, *retry, false)
		if !m.settleRuntimeDriveGateAtOwnerExit(ctx, settlement) {
			// A claimed generation retains its persisted running record; a
			// generation-zero drive with no successful lease CAS retains this exact
			// gate plus a lifecycle-fenced settlement wake. The next short pump task
			// fresh-loads and persists or mounts the successor; a process crash falls
			// back to startup recovery. Releasing here would allow a concurrent
			// second drive without durable authority.
			slog.Warn("workorch Runtime retaining drive gate after recovery lease persistence or mount failure", "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "source", request.source)
		}
		return
	}
	// A failed renewal CAS must not be followed by release of the running
	// generation: that generation is the only durable recovery authority left.
	// If persistence succeeded but the local timer was rejected, the replacement
	// scheduled generation is left intact for the settlement wake or fresh-start
	// recovery.
	if request.leaseGeneration != 0 {
		if _, err := m.runtimeReleaseDriveLease(ctx, request.workspaceID, request.runID, request.leaseGeneration); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch Runtime drive lease release failed", "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "err", err)
		}
	}
	// Release success, pre-commit failure and post-commit response loss share one
	// fresh-load settlement. Exact running G is recovered to G+1; an already
	// scheduled successor is mounted; only a confirmed no-lease result may
	// release the process gate.
	settlement := runtimeDriveGateSettlementForScheduledRequest(request, nil, true)
	if !m.settleRuntimeDriveGateAtOwnerExit(ctx, settlement) {
		slog.Warn("workorch Runtime retaining drive gate until lease release can be settled", "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "source", request.source)
	}
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
		// A running terminal-delivery generation must first consume the stable
		// failed-G budget request; generic G->G+1 recovery would manufacture a
		// successor without that authorization. Route every interrupted running
		// lease through the one fresh-load settlement path so the terminal and
		// nonterminal cases cannot diverge.
		settlement := runtimeDriveGateSettlementForScheduledRequest(request, nil, false)
		settlement.LeaseHint, settlement.HasLeaseHint = lease.Clone(), true
		m.settleRuntimeDriveGateOrRetain(ctx, settlement)
		return true, nil
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
	failure, _ := m.runtimeScheduledDrivePanicFailure(ctx, request, recovered)
	if retry == nil {
		retry = runtimeScheduledDrivePanicLeaseRequest(request.source, failure.PersistedRevision, failure.RevisionKnown)
	}
	settlement := runtimeDriveGateSettlementForScheduledRequest(request, retry, retry == nil)
	if retry != nil && settlement.Retry.FromRunningGeneration == 0 && request.leaseGeneration != 0 {
		settlement.Retry.FromRunningGeneration = request.leaseGeneration
	}
	defer func() {
		if secondary := recover(); secondary != nil {
			m.retainRuntimeDriveGateSettlement(settlement)
			slog.Error("workorch Runtime scheduled drive exit recovery panic retained owner gate", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration, "panicType", fmt.Sprintf("%T", secondary))
		}
	}()
	if !m.settleRuntimeDriveGateAtOwnerExit(ctx, settlement) {
		slog.Warn("workorch Runtime exit panic retained gate for lifecycle settlement", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "generation", request.leaseGeneration)
	}
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

func (m *Biz) handleScheduledRuntimeDriveFailure(ctx context.Context, request scheduledRuntimeDriveRequest, err error) (*runtimeRecoveryLeaseRequest, bool) {
	if err == nil {
		return nil, false
	}
	// A cross-owner await can fail after another Orchestrator-private event has
	// already persisted terminal authority (most notably administrative
	// termination). Rebase once at the scheduled-worker boundary so every drive
	// error shares the same SM-4 rule: current terminal authority wins over a
	// stale in-flight error. A terminal-delivery error is already on its lane and
	// must not be eagerly repeated here. The explicit handled result distinguishes
	// a completed terminal rebase (nil retry) from an unclassified error, so panic
	// and preflight callers cannot manufacture a fallback lease afterward.
	var terminalDeliveryErr *runtimeTerminalDeliveryError
	if !errors.As(err, &terminalDeliveryErr) {
		if retry, handled := m.rebaseScheduledRuntimeDriveFailureToCurrentTerminal(ctx, request, err); handled {
			return retry, true
		}
	}
	var preflight *runtimePersistedLeasePreflightError
	if errors.As(err, &preflight) {
		return runtimePersistedLeasePreflightRequest(request.source, preflight.Point), true
	}
	var panicFailure *runtimeScheduledDrivePanicError
	if errors.As(err, &panicFailure) {
		return runtimeScheduledDrivePanicLeaseRequest(request.source, panicFailure.PersistedRevision, panicFailure.RevisionKnown), true
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
		}, true
	}
	if operationID, claimID, deadlineText, inFlight := runtime.RuntimeOperationRecoveryInFlight(err); inFlight {
		deadline, parseErr := time.Parse(time.RFC3339Nano, deadlineText)
		if parseErr != nil {
			return nil, true
		}
		delay := time.Until(deadline)
		if delay <= 0 {
			delay = time.Millisecond
		}
		return &runtimeRecoveryLeaseRequest{
			Reason:           runtimecontract.RuntimeDriveLeaseReasonOperationRetry,
			FailureSignature: "operation-recovery-in-flight/" + operationID + "/" + claimID,
			Delay:            delay, Source: request.source,
		}, true
	}
	var persisted *runtimePersistedDrivePendingError
	if errors.As(err, &persisted) {
		slog.Debug("workorch Runtime persisted bounded retry state", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "revision", persisted.Revision)
		return &runtimeRecoveryLeaseRequest{Reason: runtimecontract.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: fmt.Sprintf("persisted-pending/%d", persisted.Revision), Delay: time.Second, Source: request.source}, true
	}
	var lifecycleDeliveryErr *runtimeLifecycleOwnerDeliveryError
	if errors.As(err, &lifecycleDeliveryErr) {
		return m.handleRuntimeLifecycleOwnerDeliveryRetry(ctx, request, lifecycleDeliveryErr), true
	}
	if errors.As(err, &terminalDeliveryErr) {
		return m.handleRuntimeTerminalDeliveryRetry(ctx, request, err), true
	}
	slog.Warn("workorch Runtime drive failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
	failure := classifyRuntimeDriveFailure(err)
	m.observeScheduledRuntimeDriveFailure(ctx, request, failure, err)
	if !runtime.FailureClassIsBusinessTerminal(failure.Class) {
		return m.handleRuntimeNonTerminalRetry(ctx, request, failure, err), true
	}
	return m.handleRuntimeBusinessTerminalFailure(ctx, request, failure), true
}

// rebaseScheduledRuntimeDriveFailureToCurrentTerminal is the single SM-4
// error-boundary classifier for concurrent terminal progress. It does not
// infer or create terminal state: it only reloads the Orchestrator-owned
// aggregate and delivers authority that is already durably terminal.
func (m *Biz) rebaseScheduledRuntimeDriveFailureToCurrentTerminal(
	ctx context.Context,
	request scheduledRuntimeDriveRequest,
	driveErr error,
) (*runtimeRecoveryLeaseRequest, bool) {
	if m == nil || m.runtimeController == nil || driveErr == nil {
		return nil, false
	}
	aggregate, err := m.runtimeController.Load(ctx, request.workspaceID, request.runID)
	if err != nil || !runtimeTerminalDeliveryReady(aggregate) {
		return nil, false
	}
	// A fresh persisted lease is already the sole SM-4 drive authority. Only the
	// worker holding that exact running generation may continue terminal
	// delivery here; a generation-zero ingress, a scheduled generation, or a
	// stale running worker must hand the existing authority back to finish for
	// recovery/mounting instead of performing naked owner delivery beside it.
	if aggregate.DriveLease != nil &&
		(request.leaseGeneration == 0 ||
			aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning ||
			aggregate.DriveLease.RecoveryGeneration != request.leaseGeneration) {
		return runtimePersistedLeasePreflightRequest(request.source, runtimePersistedLeasePreflightPointMount), true
	}
	slog.Debug("workorch Runtime discarded stale drive error after terminal progress", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "revision", aggregate.StateRevision, "driveErr", driveErr)
	if err := m.deliverRuntimeTerminal(ctx, aggregate); err != nil {
		return m.handleRuntimeTerminalDeliveryRetry(ctx, request, runtimeTerminalDeliveryFailure(err)), true
	}
	return nil, true
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
	settlement := runtimeDriveGateSettlementForLease(lease, source)
	if !m.settleRuntimeDriveGateOrRetain(ctx, settlement) {
		return fmt.Errorf("%w: persisted lifecycle delivery lease timer was not accepted", common.ErrConflict)
	}
	return nil
}

func (m *Biz) handleRuntimeTerminalDeliveryRetry(ctx context.Context, request scheduledRuntimeDriveRequest, err error) *runtimeRecoveryLeaseRequest {
	// The final acknowledgement CAS may have committed even when its response
	// was lost. Reload before spending delivery budget: a fully acknowledged
	// current terminal needs no successor and must remain a zero-write replay.
	if m != nil && m.runtimeController != nil {
		if aggregate, loadErr := m.runtimeController.Load(ctx, request.workspaceID, request.runID); loadErr == nil &&
			runtimeTerminalDeliveryReady(aggregate) && runtimeTerminalDeliveryFullyAcknowledged(aggregate) {
			slog.Debug("workorch Runtime terminal delivery response loss already acknowledged", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "revision", aggregate.StateRevision)
			return nil
		}
	}
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: request.workspaceID, RunID: request.runID, Kind: orchestratorevents.RuntimeDriveObservationDeliveryPending,
		RecoveryGeneration: request.leaseGeneration, FailureCategory: "terminal_delivery_pending", LeaseState: runtimeDriveLeaseStateForGeneration(request.leaseGeneration), Source: request.source,
	})
	slog.Warn("workorch Runtime terminal delivery failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
	recoveryRequest, consumed, settled := m.consumeRuntimeDeliveryRetryBudget(ctx, request)
	if consumed {
		return runtimeDeliveryRecoveryLease(request.source, "terminal-delivery", recoveryRequest.RequestID)
	}
	if !settled {
		return runtimeTerminalDeliveryBudgetPendingGate(request.source, recoveryRequest, request.leaseGeneration)
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
	pending := &runtimeRecoveryLeaseRequest{
		Source: request.source, FromRunningGeneration: request.leaseGeneration,
		NonTerminalRecoveryCause: driveErr, AwaitingNonTerminalRecovery: true,
	}
	if m == nil || m.runtimeController == nil {
		return pending
	}
	aggregate, err := m.runtimeController.Load(ctx, request.workspaceID, request.runID)
	if err != nil {
		slog.Warn("workorch Runtime nonterminal recovery admission load failed", "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		return pending
	}
	recoveryRequest, err := runtime.NewRuntimeNonTerminalRecoveryRequest(
		request.workspaceID,
		request.runID,
		request.leaseGeneration,
		aggregate.StateRevision,
		time.Now().UTC().Format(time.RFC3339Nano),
		driveErr,
	)
	if err != nil {
		// A textual conflict, arbitrary Temporary implementation, provider error
		// or unbound operation timeout cannot acquire generic SM-4 retry
		// authority. Persist a contract defect instead of falling back to an
		// unbudgeted lease assembled from the error string.
		contractFailure := runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment,
			Code:  "runtime_nonterminal_recovery_untyped", Signature: "runtime-nonterminal-recovery-untyped",
			Message: "runtime nonterminal recovery requires a typed persistence outcome",
		}
		return m.handleRuntimeBusinessTerminalFailure(ctx, request, contractFailure)
	}
	pending.NonTerminalRecoveryRequest = recoveryRequest
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:  runtime.RuntimeInternalEventNonTerminalRecoveryScheduled,
		Scope: runtime.RuntimeInternalEventScope{WorkspaceID: request.workspaceID, RunID: request.runID},
		Delivery: &runtime.RuntimeInternalDeliveryPayload{
			NonTerminalRecoveryRequest: &recoveryRequest,
			NonTerminalRecoveryCause:   driveErr,
		},
	})
	if err == nil && runtimeNonTerminalRecoveryResultMatches(result, recoveryRequest) {
		return pending
	}
	// Save may have committed before its response was lost. A fresh exact
	// record is the only proof of success; otherwise the same process-local
	// request remains behind the gate for lifecycle-fenced reconciliation.
	if fresh, loadErr := m.runtimeController.Load(ctx, request.workspaceID, request.runID); loadErr == nil &&
		runtimeNonTerminalRecoveryAggregateMatches(fresh, recoveryRequest) {
		return pending
	}
	if err == nil {
		err = common.ErrInvalidArgument
	}
	slog.Warn("workorch Runtime nonterminal recovery transaction unresolved", "workspaceID", request.workspaceID, "runID", request.runID, "requestID", recoveryRequest.RequestID, "err", err)
	return pending
}

func runtimeNonTerminalRecoveryResultMatches(result runtime.RuntimeInternalEventResult, request runtimecontract.RuntimeNonTerminalRecoveryRequest) bool {
	if result.NonTerminalRecoveryRecord == nil ||
		!runtimecontract.SameRuntimeNonTerminalRecoveryRequestIdentity(result.NonTerminalRecoveryRecord.Request, request) {
		return false
	}
	return runtimeNonTerminalRecoveryAggregateMatches(result.Aggregate, request)
}

func runtimeNonTerminalRecoveryAggregateMatches(aggregate runtime.RuntimeAggregate, request runtimecontract.RuntimeNonTerminalRecoveryRequest) bool {
	record := aggregate.Facts.Recovery.Normalized().NonTerminalRecovery
	return runtimecontract.RuntimeNonTerminalRecoveryRecordPresent(record) &&
		runtimecontract.SameRuntimeNonTerminalRecoveryRequestIdentity(record.Request, request)
}

func (m *Biz) handleRuntimeBusinessTerminalFailure(ctx context.Context, request scheduledRuntimeDriveRequest, failure runtimecontract.FailureFact) *runtimeRecoveryLeaseRequest {
	recorded, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventDriveFailureObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: request.workspaceID, RunID: request.runID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{FailureFact: &failure},
	})
	if err != nil {
		slog.Warn("workorch Runtime drive failure fact commit failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		persistenceFailure := classifyRuntimeDriveFailure(err)
		if !runtime.FailureClassIsBusinessTerminal(persistenceFailure.Class) {
			// The failure-fact writer is itself behind the same Run-owner CAS
			// boundary as every other SM-4 transition. A typed CAS/owner outcome
			// must therefore acquire retry authority through the canonical atomic
			// nonterminal recovery transaction; assembling a cas_reload lease here
			// would split budget and successor ownership across two transitions.
			return m.handleRuntimeNonTerminalRetry(ctx, request, persistenceFailure, err)
		}
		return runtimeFailureFactPendingLeaseRequest(request.source, failure)
	}
	aggregate := recorded.Aggregate
	if aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || aggregate.Decision.Terminal == nil {
		slog.Warn("workorch Runtime drive failure did not reduce to terminal", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID)
		return runtimeFailureFactPendingLeaseRequest(request.source, failure)
	}
	if err := m.deliverRuntimeTerminal(ctx, aggregate); err != nil {
		m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
			WorkspaceID: request.workspaceID, RunID: request.runID, Kind: orchestratorevents.RuntimeDriveObservationDeliveryPending,
			AggregateRevision: aggregate.StateRevision, RecoveryGeneration: request.leaseGeneration, FailureCategory: "terminal_delivery_pending", LeaseState: runtimeDriveLeaseStateForGeneration(request.leaseGeneration), Source: request.source,
		})
		slog.Warn("workorch Runtime drive failure delivery failed", "source", request.source, "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		recoveryRequest, consumed, settled := m.consumeRuntimeDeliveryRetryBudget(ctx, request)
		if consumed {
			return runtimeDeliveryRecoveryLease(request.source, "terminal-delivery", recoveryRequest.RequestID)
		}
		if !settled {
			return runtimeTerminalDeliveryBudgetPendingGate(request.source, recoveryRequest, request.leaseGeneration)
		}
	}
	return nil
}

const runtimeTerminalDeliveryCASRebaseLimit = 8

// consumeRuntimeDeliveryRetryBudget returns whether the bounded retry was
// consumed and whether the owner-private budget decision reached an
// authoritative linearization point. Delivery acknowledgements may win the
// same aggregate CAS; typed conflicts are therefore reloaded here rather than
// being mistaken for budget exhaustion. An unresolved error is distinct from
// a clean "not consumed" result so the caller cannot release its sole drive
// gate while terminal delivery is still pending.
func (m *Biz) consumeRuntimeDeliveryRetryBudget(ctx context.Context, request scheduledRuntimeDriveRequest, existing ...runtimecontract.RuntimeTerminalDeliveryRecoveryRequest) (runtimecontract.RuntimeTerminalDeliveryRecoveryRequest, bool, bool) {
	if m == nil || m.runtimeController == nil {
		return runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}, false, false
	}
	if len(existing) > 1 {
		return runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}, false, false
	}
	var recoveryRequest runtimecontract.RuntimeTerminalDeliveryRecoveryRequest
	if len(existing) == 1 {
		recoveryRequest = existing[0].Normalized()
		if recoveryRequest.Validate() != nil || recoveryRequest.FailedLeaseGeneration != request.leaseGeneration {
			return runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}, false, false
		}
	} else {
		baseline, err := m.runtimeController.Load(ctx, request.workspaceID, request.runID)
		if err != nil {
			slog.Warn("workorch Runtime terminal delivery budget baseline load failed", "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
			return runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}, false, false
		}
		var deriveErr error
		recoveryRequest, deriveErr = runtimeTerminalDeliveryRecoveryRequestFromAggregate(baseline, request.leaseGeneration)
		if deriveErr != nil {
			return runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}, false, true
		}
	}
	for attempt := 0; attempt < runtimeTerminalDeliveryCASRebaseLimit; attempt++ {
		current, consumed, err := m.runtimeConsumeTerminalDeliveryRecoveryBudget(ctx, request.workspaceID, request.runID, recoveryRequest)
		if err == nil {
			if current.Facts.Recovery.Normalized().TerminalDeliveryRequest == recoveryRequest {
				return recoveryRequest, true, true
			}
			return recoveryRequest, consumed, true
		}
		// Repository adapters may report a transport/storage response error even
		// after the owner CAS committed. Re-read before classifying the error: a
		// ledger advance is this request's settled authority, while a final owner
		// acknowledgement makes another attempt unnecessary.
		if current, loadErr := m.runtimeController.Load(ctx, request.workspaceID, request.runID); loadErr == nil {
			if current.Facts.Recovery.Normalized().TerminalDeliveryRequest == recoveryRequest {
				return recoveryRequest, true, true
			}
			if runtimeTerminalDeliveryReady(current) && runtimeTerminalDeliveryFullyAcknowledged(current) {
				return recoveryRequest, false, true
			}
		}
		var cas *runtime.RuntimeCASConflictError
		if errors.As(err, &cas) {
			continue
		}
		slog.Warn("workorch Runtime terminal delivery budget commit failed", "workspaceID", request.workspaceID, "runID", request.runID, "err", err)
		return recoveryRequest, false, false
	}
	slog.Warn("workorch Runtime terminal delivery budget CAS did not settle", "workspaceID", request.workspaceID, "runID", request.runID, "attempts", runtimeTerminalDeliveryCASRebaseLimit)
	return recoveryRequest, false, false
}

func runtimeTerminalDeliveryRecoveryRequestFromAggregate(aggregate runtime.RuntimeAggregate, failedGeneration uint64) (runtimecontract.RuntimeTerminalDeliveryRecoveryRequest, error) {
	outcome, terminal, err := runtimecontract.DeriveRuntimeTerminalOutcome(aggregate.Decision, aggregate.Termination)
	if err != nil || !terminal {
		return runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}, fmt.Errorf("%w: Runtime aggregate has no terminal delivery identity", common.ErrConflict)
	}
	request := runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{
		TerminalDeliveryID: runtimecontract.StableTerminalDeliveryID(
			aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthoritySnapshot.ID, outcome,
		),
		FailedLeaseGeneration: failedGeneration,
	}
	request.RequestID = runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(request.TerminalDeliveryID, failedGeneration)
	if err := request.Validate(); err != nil {
		return runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}, err
	}
	return request, nil
}

func runtimeTerminalDeliveryBudgetPendingGate(source string, request runtimecontract.RuntimeTerminalDeliveryRecoveryRequest, failedGeneration uint64) *runtimeRecoveryLeaseRequest {
	return &runtimeRecoveryLeaseRequest{
		Source: source, TerminalDeliveryRecoveryRequest: request.Normalized(),
		AwaitingTerminalDeliveryBudget: true, TerminalDeliveryFailedGeneration: failedGeneration,
	}
}

func runtimeDeliveryRecoveryLease(source, signature string, recoveryRequestIDs ...string) *runtimeRecoveryLeaseRequest {
	request := &runtimeRecoveryLeaseRequest{Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: signature, Delay: time.Second, Source: source}
	if len(recoveryRequestIDs) == 1 {
		request.RecoveryRequestID = strings.TrimSpace(recoveryRequestIDs[0])
	}
	return request
}

// runtimeFailureFactPendingLeaseRequest preserves SM-4 successor authority
// when the owner-private failure transition has not reached its CAS boundary.
// This is not a delivery retry: no terminal authority exists until the exact
// failure fact and any active-dispatch invalidation commit together. Typed
// Run-owner persistence outcomes are handled by the atomic nonterminal
// recovery transaction before this helper is reached; this fallback is only a
// stable persisted-pending re-drive that fresh startup can adopt.
func runtimeFailureFactPendingLeaseRequest(source string, failure runtimecontract.FailureFact) *runtimeRecoveryLeaseRequest {
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
	Reason                          runtimecontract.RuntimeDriveLeaseReason
	FailureSignature                string
	RecoveryRequestID               string
	Delay                           time.Duration
	Source                          string
	FromRunningGeneration           uint64
	TerminalDeliveryRecoveryRequest runtimecontract.RuntimeTerminalDeliveryRecoveryRequest
	AwaitingTerminalDeliveryBudget  bool
	// NonTerminalRecoveryRequest and Cause are process-local retry inputs for
	// the single owner-private event that atomically settles the technical
	// budget and its successor lease. They never cross EventHub and never become
	// a second persisted authority; every retry is re-admitted against a fresh
	// aggregate revision.
	NonTerminalRecoveryRequest  runtimecontract.RuntimeNonTerminalRecoveryRequest
	NonTerminalRecoveryCause    error
	AwaitingNonTerminalRecovery bool
	// TerminalDeliveryFailedGeneration distinguishes a delivery attempt whose
	// budget CAS is unresolved from a fresh terminal that has not yet been sent.
	// Zero is a valid generation-zero failure and is therefore interpreted only
	// when AwaitingTerminalDeliveryBudget is true.
	TerminalDeliveryFailedGeneration uint64
}

// runtimeDriveGateSettlementRequest is process-local wakeup input for the one
// SM-4 exit reconciler. It cannot authorize a drive: Retry may only request the
// existing owner-private lease-schedule event, while LeaseHint is used solely
// to keep an unmounted generation visible until the next fresh aggregate load.
type runtimeDriveGateSettlementRequest struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	DriveKey           string
	GateEpoch          uint64
	GateToken          uint64
	Source             string
	Retry              runtimeRecoveryLeaseRequest
	HasRetry           bool
	LeaseHint          runtimecontract.RuntimeDriveLease
	HasLeaseHint       bool
	ReleaseWhenNoLease bool
	// AwaitGateOwnerExit is an owner-local handoff fence. A concurrent
	// administrative termination may persist while an older worker still owns
	// this gate; the lifecycle wake must remain dormant until that exact worker
	// reaches its exit boundary. It is never persisted Runtime authority.
	AwaitGateOwnerExit bool
}

func runtimeDriveGateSettlementForScheduledRequest(request scheduledRuntimeDriveRequest, retry *runtimeRecoveryLeaseRequest, releaseWhenNoLease bool) runtimeDriveGateSettlementRequest {
	settlement := runtimeDriveGateSettlementRequest{
		WorkspaceID: request.workspaceID, RunID: request.runID, DriveKey: request.driveKey,
		GateEpoch: request.gateEpoch, GateToken: request.gateToken,
		Source: request.source, ReleaseWhenNoLease: releaseWhenNoLease,
	}
	if retry != nil {
		settlement.Retry = *retry
		settlement.HasRetry = true
		settlement.ReleaseWhenNoLease = false
	}
	return settlement
}

// runtimeScheduledDriveRequestActive prevents an accepted-but-queued callback
// from crossing Stop/Start or a same-epoch gate release/reacquire. The exact
// gate identity is captured before BackgroundRoutine submission and is never
// rediscovered from current process state by the callback.
func (m *Biz) runtimeScheduledDriveRequestActive(request scheduledRuntimeDriveRequest) bool {
	if m == nil || request.driveKey == "" || request.gateEpoch == 0 || request.gateToken == 0 {
		return false
	}
	m.driveMu.Lock()
	gate, ownsGate := m.drivingRuns[request.driveKey]
	active := !m.runtimeLeaseTimerClosing && m.runtimeLeaseTimerEpoch == request.gateEpoch && ownsGate &&
		gate.Epoch == request.gateEpoch && gate.Token == request.gateToken
	m.driveMu.Unlock()
	return active
}

func runtimeDriveGateSettlementForLease(lease runtimecontract.RuntimeDriveLease, source string) runtimeDriveGateSettlementRequest {
	return runtimeDriveGateSettlementRequest{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, DriveKey: runtimeDriveKey(lease.WorkspaceID, lease.RunID), Source: source,
		LeaseHint: lease.Clone(), HasLeaseHint: true,
	}
}

type runtimeRecoveryLeaseScheduleDisposition uint8

const (
	runtimeRecoveryLeaseScheduleFailed runtimeRecoveryLeaseScheduleDisposition = iota
	runtimeRecoveryLeaseScheduleMounted
	runtimeRecoveryLeaseScheduleTerminalComplete
)

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

// scheduleRuntimeRecoveryLease persists retry identity before the timer is
// started. Returning true means the caller must retain its singleflight gate.
func (m *Biz) scheduleRuntimeRecoveryLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, request runtimeRecoveryLeaseRequest) bool {
	return m.scheduleRuntimeRecoveryLeaseDisposition(ctx, workspaceID, runID, request) == runtimeRecoveryLeaseScheduleMounted
}

func (m *Biz) scheduleRuntimeRecoveryLeaseDisposition(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, request runtimeRecoveryLeaseRequest) runtimeRecoveryLeaseScheduleDisposition {
	if m == nil || m.runtimeController == nil || workspaceID == "" || runID == "" || request.Reason == "" || request.FailureSignature == "" {
		return runtimeRecoveryLeaseScheduleFailed
	}
	if request.Delay <= 0 {
		request.Delay = time.Second
	}
	var result runtime.RuntimeInternalEventResult
	var err error
	terminalDelivery := request.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry && request.FailureSignature == "terminal-delivery"
	for attempt := 0; attempt < runtimeTerminalDeliveryCASRebaseLimit; attempt++ {
		result, err = m.applyRuntimeDriveLeaseSchedule(ctx, workspaceID, runID, runtimecontract.RuntimeDriveLease{
			WorkspaceID: workspaceID, RunID: runID, Reason: request.Reason, FailureSignature: request.FailureSignature,
			RecoveryRequestID: request.RecoveryRequestID, DueAt: time.Now().UTC().Add(request.Delay).Format(time.RFC3339Nano),
		}, request.FromRunningGeneration)
		if err == nil {
			break
		}
		// A lease CAS or the exact fully-acknowledged running-lease cleanup can
		// commit while its response is lost. Rebase generic as well as typed CAS
		// errors onto the current terminal authority before deciding to retain the
		// gate. A persisted scheduled terminal lease is already the successor;
		// an absent lease after final acknowledgement is already complete.
		if terminalDelivery {
			if current, loadErr := m.runtimeController.Load(ctx, workspaceID, runID); loadErr == nil {
				if runtimeTerminalDeliveryFullyAcknowledged(current) && current.DriveLease == nil {
					result = runtime.RuntimeInternalEventResult{Aggregate: current}
					err = nil
					break
				}
				if current.DriveLease != nil && current.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled &&
					current.DriveLease.Reason == request.Reason && current.DriveLease.FailureSignature == request.FailureSignature {
					lease := current.DriveLease.Clone()
					result = runtime.RuntimeInternalEventResult{Aggregate: current, DriveLease: &lease}
					err = nil
					break
				}
				if runtimeTerminalDeliveryFullyAcknowledged(current) && current.DriveLease != nil &&
					current.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning &&
					current.DriveLease.RecoveryGeneration == request.FromRunningGeneration {
					// The cleanup did not commit but the repository is readable; the
					// same generation-fenced owner CAS may be retried safely.
					continue
				}
			}
		}
		var cas *runtime.RuntimeCASConflictError
		if !terminalDelivery || !errors.As(err, &cas) {
			break
		}
	}
	if err != nil {
		slog.Warn("workorch Runtime recovery lease commit failed", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "reason", request.Reason, "signature", request.FailureSignature, "err", err)
		return runtimeRecoveryLeaseScheduleFailed
	}
	if result.DriveLease == nil {
		if request.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry && request.FailureSignature == "terminal-delivery" &&
			runtimeTerminalDeliveryFullyAcknowledged(result.Aggregate) {
			slog.Debug("workorch Runtime recovery lease omitted after terminal delivery completed", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "revision", result.Aggregate.StateRevision)
			return runtimeRecoveryLeaseScheduleTerminalComplete
		}
		return runtimeRecoveryLeaseScheduleFailed
	}
	lease := result.DriveLease.Clone()
	if lease.State != runtimecontract.RuntimeDriveLeaseScheduled || !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, request.Source) {
		slog.Warn("workorch Runtime recovery lease timer was not accepted", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "generation", lease.RecoveryGeneration)
		return runtimeRecoveryLeaseScheduleFailed
	}
	m.clearPersistedRuntimeDriveLeaseMount(runtimeDriveKey(workspaceID, runID), lease.RecoveryGeneration)
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: request.Source,
	})
	slog.Info("workorch Runtime recovery lease scheduled", "source", request.Source, "workspaceID", workspaceID, "runID", runID, "revision", lease.AggregateRevision, "generation", lease.RecoveryGeneration, "reason", lease.Reason, "signature", lease.FailureSignature, "state", lease.State, "dueAt", lease.DueAt)
	return runtimeRecoveryLeaseScheduleMounted
}

// reconcileRuntimeDriveGateSettlement is the only non-Stop exit settlement
// for a run gate. It never trusts process-local timer/latch state as Runtime
// authority: every attempt fresh-loads the Run aggregate, adopts scheduled
// authority exactly, or converts the current running G to scheduled G+1 only
// through the Orchestrator-private recovery event. A false result means the
// caller must leave the gate in place and let the lifecycle pump retry.
func (m *Biz) reconcileRuntimeDriveGateSettlement(ctx context.Context, request runtimeDriveGateSettlementRequest) bool {
	if m == nil || m.runtimeController == nil || request.WorkspaceID == "" || request.RunID == "" ||
		request.DriveKey != runtimeDriveKey(request.WorkspaceID, request.RunID) || request.GateEpoch == 0 || request.GateToken == 0 ||
		!m.runtimeDriveGateMatches(request.DriveKey, request.GateEpoch, request.GateToken) {
		return false
	}
	aggregate, err := m.runtimeController.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return false
	}
	if errors.Is(err, common.ErrNotFound) {
		return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
	}

	// A fully acknowledged terminal has no remaining delivery authority. Clear
	// only the exact running generation before considering budget settlement or
	// ordinary interrupted-running recovery; a response-loss reload proves the
	// same result without manufacturing G+1.
	if runtimeTerminalDeliveryReady(aggregate) && runtimeTerminalDeliveryFullyAcknowledged(aggregate) {
		settledAggregate, settled := m.settleFullyAcknowledgedRuntimeDriveLease(ctx, aggregate)
		if !settled {
			return false
		}
		aggregate = settledAggregate
		if aggregate.DriveLease != nil {
			// A fully acknowledged terminal has no legal scheduled or running
			// successor. Never remount it; a fresh exact owner event must clear it.
			return false
		}
		return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
	}
	if aggregate.DriveLease == nil && request.ReleaseWhenNoLease {
		// A successful synchronous DriveRuntime is reducer-only; its caller owns
		// the normal projection/delivery phase. Once fresh state proves there is no
		// durable successor, exit settlement must not infer a second terminal
		// delivery or recovery budget from that clean outcome.
		return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
	}
	if runtimeTerminalDeliveryReady(aggregate) {
		// Terminal authority always supersedes a stale process-local nonterminal
		// retry. This one lane distinguishes a first owner delivery from an exact
		// failed-generation budget settlement and is the only place that may mint
		// the corresponding canonical successor.
		return m.reconcileRuntimeTerminalDeliveryAtGate(ctx, aggregate, request)
	}
	if request.HasRetry && request.Retry.AwaitingNonTerminalRecovery {
		return m.settleRuntimeNonTerminalRecoveryAtGate(ctx, aggregate, request)
	}

	if request.HasRetry {
		retry := request.Retry
		if aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
			return m.mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx, request, aggregate.DriveLease.Clone(), request.Source+":retry_successor")
		}
		if aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning {
			if retry.FromRunningGeneration == 0 || retry.FromRunningGeneration != aggregate.DriveLease.RecoveryGeneration {
				return false
			}
		}
		switch m.scheduleRuntimeRecoveryLeaseDisposition(ctx, request.WorkspaceID, request.RunID, retry) {
		case runtimeRecoveryLeaseScheduleMounted:
			return true
		case runtimeRecoveryLeaseScheduleTerminalComplete:
			return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
		default:
			return false
		}
	}

	if aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning {
		for attempt := 0; attempt < runtimeTerminalDeliveryCASRebaseLimit; attempt++ {
			runningGeneration := aggregate.DriveLease.RecoveryGeneration
			recovered, _, recoverErr := m.runtimeRecoverDriveLease(ctx, request.WorkspaceID, request.RunID, time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano))
			if recoverErr == nil && recovered.WorkspaceID != "" {
				if recovered.Validate() != nil || recovered.State != runtimecontract.RuntimeDriveLeaseScheduled || recovered.RecoveryGeneration != runningGeneration+1 {
					return false
				}
				aggregate.DriveLease = &recovered
				break
			}
			fresh, loadErr := m.runtimeController.Load(ctx, request.WorkspaceID, request.RunID)
			if loadErr != nil {
				return false
			}
			aggregate = fresh
			if aggregate.DriveLease == nil || aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
				break
			}
		}
		if aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning {
			return false
		}
	}
	if aggregate.DriveLease != nil {
		lease := aggregate.DriveLease.Clone()
		if lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
			return false
		}
		return m.mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx, request, lease, request.Source+":exit_settlement")
	}
	if request.ReleaseWhenNoLease {
		return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
	}
	return m.settleActionableRuntimeWithoutDriveLease(ctx, aggregate, request)
}

// settleRuntimeNonTerminalRecoveryAtGate is the only outer adapter for the
// atomic generic persistence recovery event. It fresh-reissues the request
// fence after an indeterminate load/CAS, then mounts the event's already
// persisted successor. It never calls DriveLeaseScheduled or consumes a
// provider/operation budget.
func (m *Biz) settleRuntimeNonTerminalRecoveryAtGate(
	ctx context.Context,
	aggregate runtime.RuntimeAggregate,
	settlement runtimeDriveGateSettlementRequest,
) bool {
	retry := settlement.Retry
	if aggregate.DriveLease != nil {
		lease := aggregate.DriveLease.Clone()
		if lease.State == runtimecontract.RuntimeDriveLeaseScheduled {
			return m.mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx, settlement, lease, settlement.Source+":nonterminal_recovery")
		}
		if lease.State != runtimecontract.RuntimeDriveLeaseRunning ||
			lease.RecoveryGeneration != retry.FromRunningGeneration {
			return false
		}
	} else if retry.FromRunningGeneration != 0 {
		// A nonzero request may replace only its exact running generation. If
		// another transition retired it, the fresh aggregate's terminal or
		// durable successor path—not this stale local error—must take over.
		return false
	}
	recoveryRequest, err := runtime.NewRuntimeNonTerminalRecoveryRequest(
		settlement.WorkspaceID,
		settlement.RunID,
		retry.FromRunningGeneration,
		aggregate.StateRevision,
		time.Now().UTC().Format(time.RFC3339Nano),
		retry.NonTerminalRecoveryCause,
	)
	if err != nil {
		return false
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:  runtime.RuntimeInternalEventNonTerminalRecoveryScheduled,
		Scope: runtime.RuntimeInternalEventScope{WorkspaceID: settlement.WorkspaceID, RunID: settlement.RunID},
		Delivery: &runtime.RuntimeInternalDeliveryPayload{
			NonTerminalRecoveryRequest: &recoveryRequest,
			NonTerminalRecoveryCause:   retry.NonTerminalRecoveryCause,
		},
	})
	if err == nil && runtimeNonTerminalRecoveryResultMatches(result, recoveryRequest) {
		if result.DriveLease != nil && result.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
			return m.mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx, settlement, result.DriveLease.Clone(), settlement.Source+":nonterminal_recovery_committed")
		}
		if result.Aggregate.DriveLease != nil && result.Aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
			return m.mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx, settlement, result.Aggregate.DriveLease.Clone(), settlement.Source+":nonterminal_recovery_committed")
		}
	}
	fresh, loadErr := m.runtimeController.Load(ctx, settlement.WorkspaceID, settlement.RunID)
	if loadErr != nil {
		return false
	}
	if runtimeTerminalDeliveryReady(fresh) {
		return m.reconcileRuntimeTerminalDeliveryAtGate(ctx, fresh, settlement)
	}
	if fresh.DriveLease != nil && fresh.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
		return m.mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx, settlement, fresh.DriveLease.Clone(), settlement.Source+":nonterminal_recovery_rebased")
	}
	return false
}

func (m *Biz) reconcileRuntimeTerminalDeliveryAtGate(ctx context.Context, aggregate runtime.RuntimeAggregate, request runtimeDriveGateSettlementRequest) bool {
	recovery := aggregate.Facts.Recovery.Normalized()
	if recovery.TerminalDeliveryIntervention.Validate() == nil {
		if aggregate.DriveLease != nil {
			return false
		}
		return m.releaseRuntimeDriveGateForTerminalIntervention(ctx, request, recovery.TerminalDeliveryIntervention)
	}
	if aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
		return m.mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx, request, aggregate.DriveLease.Clone(), request.Source+":terminal_scheduled")
	}

	// An unresolved budget CAS proves that a delivery attempt failed even when
	// the response was lost before its stable request identity could be returned.
	// Derive an invalid/zero local identity again from this fresh terminal while
	// preserving the exact failed generation carried separately by the intent.
	if request.HasRetry && request.Retry.AwaitingTerminalDeliveryBudget {
		failedGeneration := request.Retry.TerminalDeliveryFailedGeneration
		recoveryRequest := request.Retry.TerminalDeliveryRecoveryRequest.Normalized()
		if recoveryRequest.Validate() != nil || recoveryRequest.FailedLeaseGeneration != failedGeneration {
			var err error
			recoveryRequest, err = runtimeTerminalDeliveryRecoveryRequestFromAggregate(aggregate, failedGeneration)
			if err != nil {
				return false
			}
		}
		return m.settleRuntimeTerminalDeliveryFailureAtGate(ctx, aggregate, request, failedGeneration, recoveryRequest)
	}

	if aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning {
		running := aggregate.DriveLease.Clone()
		canonicalTerminal := running.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry && running.FailureSignature == "terminal-delivery"
		localTerminalRetry := request.HasRetry && request.Retry.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry &&
			request.Retry.FailureSignature == "terminal-delivery" && request.Retry.RecoveryRequestID != ""
		if canonicalTerminal || localTerminalRetry {
			return m.settleRuntimeTerminalDeliveryFailureAtGate(ctx, aggregate, request, running.RecoveryGeneration, runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{})
		}
		// A terminal committed while a nonterminal worker was in flight has not
		// necessarily been delivered. Ignore that stale local retry and perform the
		// first idempotent owner delivery before spending terminal retry budget.
		return m.deliverFirstRuntimeTerminalAtGate(ctx, aggregate, request, running.RecoveryGeneration)
	}

	if aggregate.DriveLease != nil {
		return false
	}
	if authorization := recovery.TerminalDeliveryRequest; authorization.Validate() == nil {
		// A consumed R(F) with no lease is the crash/stale-claim cut before its
		// canonical F+1 schedule. Re-materialize it without another delivery or
		// another budget write.
		return m.settleRuntimeTerminalDeliveryFailureAtGate(ctx, aggregate, request, authorization.FailedLeaseGeneration, authorization)
	}
	failedGeneration := uint64(0)
	if retired := recovery.RetiredTerminalDeliveryLease; retired.Validate() == nil && retired.State == runtimecontract.RuntimeDriveLeaseRunning &&
		recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID == "" {
		failedGeneration = retired.RecoveryGeneration
	}
	return m.deliverFirstRuntimeTerminalAtGate(ctx, aggregate, request, failedGeneration)
}

func (m *Biz) settleRuntimeTerminalDeliveryFailureAtGate(
	ctx context.Context,
	aggregate runtime.RuntimeAggregate,
	request runtimeDriveGateSettlementRequest,
	failedGeneration uint64,
	existing runtimecontract.RuntimeTerminalDeliveryRecoveryRequest,
) bool {
	recoveryRequest := existing.Normalized()
	if recoveryRequest.Validate() != nil || recoveryRequest.FailedLeaseGeneration != failedGeneration {
		var err error
		recoveryRequest, err = runtimeTerminalDeliveryRecoveryRequestFromAggregate(aggregate, failedGeneration)
		if err != nil {
			return false
		}
	}
	scheduledRequest := scheduledRuntimeDriveRequest{
		workspaceID: request.WorkspaceID, runID: request.RunID, source: request.Source,
		driveKey: request.DriveKey, leaseGeneration: failedGeneration,
	}
	settledRequest, authorized, settled := m.consumeRuntimeDeliveryRetryBudget(ctx, scheduledRequest, recoveryRequest)
	if !settled {
		return false
	}
	if !authorized {
		fresh, err := m.runtimeController.Load(ctx, request.WorkspaceID, request.RunID)
		if err != nil {
			return false
		}
		if runtimeTerminalDeliveryReady(fresh) && runtimeTerminalDeliveryFullyAcknowledged(fresh) {
			completed, ok := m.settleFullyAcknowledgedRuntimeDriveLease(ctx, fresh)
			return ok && completed.DriveLease == nil && m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
		}
		return m.releaseRuntimeDriveGateForTerminalIntervention(ctx, request, settledRequest)
	}
	retry := runtimeDeliveryRecoveryLease(request.Source, "terminal-delivery", settledRequest.RequestID)
	retry.FromRunningGeneration = failedGeneration
	switch m.scheduleRuntimeRecoveryLeaseDisposition(ctx, request.WorkspaceID, request.RunID, *retry) {
	case runtimeRecoveryLeaseScheduleMounted:
		return true
	case runtimeRecoveryLeaseScheduleTerminalComplete:
		return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
	default:
		return false
	}
}

func (m *Biz) deliverFirstRuntimeTerminalAtGate(
	ctx context.Context,
	aggregate runtime.RuntimeAggregate,
	request runtimeDriveGateSettlementRequest,
	failedGeneration uint64,
) bool {
	if err := m.deliverRuntimeTerminal(ctx, aggregate); err != nil {
		return m.settleRuntimeTerminalDeliveryFailureAtGate(ctx, aggregate, request, failedGeneration, runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{})
	}
	fresh, err := m.runtimeController.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil || !runtimeTerminalDeliveryReady(fresh) || !runtimeTerminalDeliveryFullyAcknowledged(fresh) {
		return false
	}
	settled, ok := m.settleFullyAcknowledgedRuntimeDriveLease(ctx, fresh)
	if !ok || settled.DriveLease != nil {
		return false
	}
	return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
}

func (m *Biz) settleFullyAcknowledgedRuntimeDriveLease(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, bool) {
	if aggregate.DriveLease == nil {
		return aggregate, true
	}
	for attempt := 0; attempt < runtimeTerminalDeliveryCASRebaseLimit; attempt++ {
		if aggregate.DriveLease == nil {
			return aggregate, true
		}
		lease := aggregate.DriveLease.Clone()
		_, releaseErr := m.runtimeReleaseDriveLease(ctx, lease.WorkspaceID, lease.RunID, lease.RecoveryGeneration)
		fresh, loadErr := m.runtimeController.Load(ctx, lease.WorkspaceID, lease.RunID)
		if loadErr == nil {
			if fresh.DriveLease == nil {
				return fresh, true
			}
			if !runtimeTerminalDeliveryReady(fresh) || !runtimeTerminalDeliveryFullyAcknowledged(fresh) {
				return fresh, false
			}
			aggregate = fresh
		}
		if releaseErr == nil && loadErr != nil {
			return runtime.RuntimeAggregate{}, false
		}
	}
	return aggregate, false
}

func (m *Biz) settleActionableRuntimeWithoutDriveLease(ctx context.Context, aggregate runtime.RuntimeAggregate, request runtimeDriveGateSettlementRequest) bool {
	if runtimeTerminalDeliveryReady(aggregate) {
		// Terminal state is handled before local Retry by
		// reconcileRuntimeTerminalDeliveryAtGate. Reaching this helper with a
		// terminal aggregate would reintroduce a parallel budget/delivery path.
		return false
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance && m.runtimeGovernanceWaitAcknowledged(ctx, aggregate) {
		return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
	}
	// Any other no-lease nonterminal remains actionable. Persist an immediate
	// owner-private re-drive authority instead of opening the gate to naked
	// ingress or relying on the local pump as business authority.
	retry := runtimeRecoveryLeaseRequest{
		Reason:           runtimecontract.RuntimeDriveLeaseReasonPersistedPending,
		FailureSignature: fmt.Sprintf("exit-settlement/%d", aggregate.StateRevision), Delay: time.Millisecond, Source: request.Source,
	}
	return m.scheduleRuntimeRecoveryLeaseDisposition(ctx, request.WorkspaceID, request.RunID, retry) == runtimeRecoveryLeaseScheduleMounted
}

func (m *Biz) releaseRuntimeDriveGateForTerminalIntervention(
	ctx context.Context,
	request runtimeDriveGateSettlementRequest,
	recoveryRequest runtimecontract.RuntimeTerminalDeliveryRecoveryRequest,
) bool {
	aggregate, err := m.runtimeController.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil || aggregate.DriveLease != nil ||
		aggregate.Facts.Recovery.Normalized().TerminalDeliveryIntervention != recoveryRequest.Normalized() {
		return false
	}
	return m.releaseRuntimeDriveGateAfterFreshNoLease(ctx, request)
}

func (m *Biz) runtimeGovernanceWaitAcknowledged(ctx context.Context, aggregate runtime.RuntimeAggregate) bool {
	result, err := runtimeGovernedWaitDriveResult(aggregate)
	if err != nil {
		return false
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	handoff, governed, err := runtimeGovernanceLifecycleHandoffFromState(&record)
	if err != nil || !governed {
		return false
	}
	run, err := (runSnapshotReader{hub: m.EventHub()}).Read(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		return false
	}
	if (run.State != common.RunStateBlockedOnDecision || run.BlockingTicketID != result.BlockingTicketID) &&
		(run.State != common.RunStateBlockedOnApproval || run.BlockingApprovalID != result.BlockingApprovalID) {
		return false
	}
	fact, err := runGovernanceWaitFact(
		aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, handoff.AuthoritySnapshotID, handoff.DefinitionRevision,
		handoff.EvidenceRevision, handoff.DispatchID, handoff.TicketID, handoff.ApprovalID,
	)
	return err == nil && validateRunGovernanceWaitAcknowledgement(runevents.RunLifecycleCommandResult{Run: run}, fact, aggregate.StateRevision) == nil
}

// releaseRuntimeDriveGateAfterFreshNoLease makes the no-authority result and
// a concurrently latched lifecycle successor one atomic local handoff. The
// fresh load above authorizes release only when no newer latch won this lock.
func (m *Biz) releaseRuntimeDriveGateAfterFreshNoLease(ctx context.Context, request runtimeDriveGateSettlementRequest) bool {
	lease, retained, released := m.takePersistedRuntimeDriveLeaseMountExact(request.DriveKey, request.GateEpoch, request.GateToken, true)
	if !retained {
		return released
	}
	return m.mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx, request, lease, request.Source+":latched_settlement")
}

func (m *Biz) mountScheduledRuntimeDriveLeaseAtOwnedGate(ctx context.Context, request runtimeDriveGateSettlementRequest, lease runtimecontract.RuntimeDriveLease, source string) bool {
	key := request.DriveKey
	if m == nil || key == "" || key != runtimeDriveKey(lease.WorkspaceID, lease.RunID) || lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		!m.runtimeDriveGateMatches(key, request.GateEpoch, request.GateToken) {
		return false
	}
	if !m.enqueueRuntimeDriveLeaseTimer(ctx, lease, source) {
		m.retainPendingRuntimeDriveLeaseMount(key, lease)
		return false
	}
	m.clearPersistedRuntimeDriveLeaseMount(key, lease.RecoveryGeneration)
	m.observeRuntimeDrive(ctx, orchestratorevents.RuntimeDriveObservedData{
		WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerScheduled,
		AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: source,
	})
	return true
}

func (m *Biz) runtimeDriveGateMatches(key string, epoch uint64, gateToken uint64) bool {
	if m == nil || key == "" || epoch == 0 || gateToken == 0 {
		return false
	}
	m.driveMu.Lock()
	gate, ok := m.drivingRuns[key]
	m.driveMu.Unlock()
	return ok && gate.Epoch == epoch && gate.Token == gateToken
}

// retainPendingRuntimeDriveLeaseMount marks an exact, currently unmounted
// generation behind the already-owned gate. The paired registration+latch is
// only a delivery wake hint: beginRuntimeLeaseTimer can distinguish it from a
// live timer and the one-shot settlement wake will fresh-load before retrying.
func (m *Biz) retainPendingRuntimeDriveLeaseMount(key string, lease runtimecontract.RuntimeDriveLease) bool {
	if m == nil || key == "" || key != runtimeDriveKey(lease.WorkspaceID, lease.RunID) || lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.runtimeLeaseTimerClosing {
		return false
	}
	gate, ownsGate := m.drivingRuns[key]
	if !ownsGate || gate.Epoch == 0 || gate.Token == 0 || gate.Epoch != m.runtimeLeaseTimerEpoch {
		return false
	}
	if m.persistedLeaseMounts == nil {
		m.persistedLeaseMounts = map[string]runtimecontract.RuntimeDriveLease{}
	}
	if current, ok := m.persistedLeaseMounts[key]; ok {
		if current.RecoveryGeneration > lease.RecoveryGeneration ||
			(current.RecoveryGeneration == lease.RecoveryGeneration && !runtimecontract.SameRuntimeDriveLeaseIdentity(current, lease)) {
			return false
		}
	}
	if m.retryLeaseTimers == nil {
		m.retryLeaseTimers = map[string]runtimeLeaseTimerRegistration{}
	}
	if current, ok := m.retryLeaseTimers[key]; ok && current.Epoch == gate.Epoch && current.GateToken == gate.Token {
		if current.Generation > lease.RecoveryGeneration ||
			(current.Generation == lease.RecoveryGeneration && !runtimecontract.SameRuntimeDriveLeaseIdentity(current.Lease, lease)) {
			return false
		}
	}
	m.persistedLeaseMounts[key] = lease.Clone()
	m.retryLeaseTimers[key] = runtimeLeaseTimerRegistration{Generation: lease.RecoveryGeneration, Epoch: gate.Epoch, GateToken: gate.Token, Lease: lease.Clone()}
	return true
}

func mergeRuntimeDriveGateSettlementRequest(current, next runtimeDriveGateSettlementRequest) runtimeDriveGateSettlementRequest {
	merged := next
	if merged.Source == "" {
		merged.Source = current.Source
	}
	if !merged.HasRetry && current.HasRetry {
		merged.Retry, merged.HasRetry = current.Retry, true
	}
	if current.HasLeaseHint && (!merged.HasLeaseHint || current.LeaseHint.RecoveryGeneration > merged.LeaseHint.RecoveryGeneration) {
		merged.LeaseHint, merged.HasLeaseHint = current.LeaseHint.Clone(), true
	}
	if current.HasRetry || next.HasRetry {
		merged.ReleaseWhenNoLease = false
	} else {
		merged.ReleaseWhenNoLease = current.ReleaseWhenNoLease && next.ReleaseWhenNoLease
	}
	// A dormant handoff becomes runnable as soon as either the exact gate owner
	// exits or another already-active settlement has taken responsibility for
	// the gate. Never let a later handoff put an active settlement back to
	// sleep; there may no longer be a worker capable of activating it again.
	merged.AwaitGateOwnerExit = current.AwaitGateOwnerExit && next.AwaitGateOwnerExit
	return merged
}

// retainRuntimeDriveGateSettlement registers a lifecycle/epoch-fenced local
// wake hint. The StartContext-installed pump is independent of the one-shot
// AsyncFunction whose rejection caused the settlement, so a durable lease is
// never left with a gate but no future in-process wakeup.
func (m *Biz) retainRuntimeDriveGateSettlement(request runtimeDriveGateSettlementRequest) bool {
	if m == nil || request.WorkspaceID == "" || request.RunID == "" || request.DriveKey != runtimeDriveKey(request.WorkspaceID, request.RunID) {
		return false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.runtimeLeaseTimerClosing {
		return false
	}
	gate, ownsGate := m.drivingRuns[request.DriveKey]
	if !ownsGate || gate.Epoch == 0 || gate.Token == 0 || gate.Epoch != m.runtimeLeaseTimerEpoch {
		return false
	}
	if (request.GateEpoch != 0 && request.GateEpoch != gate.Epoch) || (request.GateToken != 0 && request.GateToken != gate.Token) {
		return false
	}
	request.GateEpoch, request.GateToken = gate.Epoch, gate.Token
	if m.runtimeDriveGateSettlements == nil {
		m.runtimeDriveGateSettlements = map[string]runtimeDriveGateSettlementRegistration{}
	}
	if current, ok := m.runtimeDriveGateSettlements[request.DriveKey]; ok && current.Epoch == gate.Epoch && current.GateToken == gate.Token {
		request = mergeRuntimeDriveGateSettlementRequest(current.Request, request)
	}
	m.runtimeDriveGateSettlementToken++
	if m.runtimeDriveGateSettlementToken == 0 {
		m.runtimeDriveGateSettlementToken++
	}
	m.runtimeDriveGateSettlements[request.DriveKey] = runtimeDriveGateSettlementRegistration{Epoch: gate.Epoch, GateToken: gate.Token, Token: m.runtimeDriveGateSettlementToken, Request: request}
	m.armRuntimeDriveGateSettlementWakeLocked(runtimeDriveGateSettlementWakeDelay)
	return true
}

// retainOrAcquireRuntimeDriveGateSettlement closes the handoff window where
// the previously observed gate owner exits immediately before a termination
// or governance finalizer installs its wake hint. A newly acquired gate is
// settlement-owned and therefore active; an existing worker-owned gate keeps
// the caller's dormant owner-exit fence.
func (m *Biz) retainOrAcquireRuntimeDriveGateSettlement(request runtimeDriveGateSettlementRequest) bool {
	if m == nil {
		return false
	}
	if m.retainRuntimeDriveGateSettlement(request) {
		return true
	}
	gateToken, acquired := m.beginRuntimeDriveWithToken(request.DriveKey)
	gate, ownsGate := m.currentRuntimeDriveGate(request.DriveKey)
	if !ownsGate || gate.Token == 0 || (acquired && gate.Token != gateToken) {
		return false
	}
	request.GateEpoch, request.GateToken = gate.Epoch, gate.Token
	request.AwaitGateOwnerExit = request.AwaitGateOwnerExit && !acquired
	return m.retainRuntimeDriveGateSettlement(request)
}

func (m *Biz) settleRuntimeDriveGateOrRetain(ctx context.Context, request runtimeDriveGateSettlementRequest) bool {
	if request.GateEpoch == 0 || request.GateToken == 0 {
		gate, ok := m.currentRuntimeDriveGate(request.DriveKey)
		if !ok {
			return false
		}
		request.GateEpoch, request.GateToken = gate.Epoch, gate.Token
	}
	// The active settlement is claimed below before owner I/O so a successful
	// fresh decision can release the gate atomically. Restore that exact claim
	// before propagating a panic to the lifecycle cycle fence; otherwise the
	// cycle has no pending entry from which to re-arm its one-shot wake.
	defer func() {
		if panicValue := recover(); panicValue != nil {
			if request.HasLeaseHint {
				m.retainPendingRuntimeDriveLeaseMount(request.DriveKey, request.LeaseHint)
			}
			m.retainRuntimeDriveGateSettlement(request)
			panic(panicValue)
		}
	}()
	if pending, claimed := m.takeRuntimeDriveGateSettlementForReconcile(request.DriveKey, request.GateEpoch, request.GateToken); claimed {
		request = mergeRuntimeDriveGateSettlementRequest(pending, request)
		request.AwaitGateOwnerExit = false
	}
	if m.reconcileRuntimeDriveGateSettlement(ctx, request) {
		return true
	}
	if request.HasLeaseHint {
		m.retainPendingRuntimeDriveLeaseMount(request.DriveKey, request.LeaseHint)
	}
	m.retainRuntimeDriveGateSettlement(request)
	return false
}

// takeRuntimeDriveGateSettlementForReconcile temporarily claims the exact
// active local wake before owner I/O. This lets a successful fresh no-authority
// decision release its own gate, while any handoff registered concurrently
// during that I/O remains visible to the final release and therefore keeps the
// gate. A dormant owner-exit handoff is never claimed by the one-shot wake task.
func (m *Biz) takeRuntimeDriveGateSettlementForReconcile(key string, epoch uint64, gateToken uint64) (runtimeDriveGateSettlementRequest, bool) {
	if m == nil || key == "" || epoch == 0 || gateToken == 0 {
		return runtimeDriveGateSettlementRequest{}, false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	current, ok := m.runtimeDriveGateSettlements[key]
	if !ok || current.Epoch != epoch || current.GateToken != gateToken || current.Request.AwaitGateOwnerExit {
		return runtimeDriveGateSettlementRequest{}, false
	}
	delete(m.runtimeDriveGateSettlements, key)
	return current.Request, true
}

// settleRuntimeDriveGateAtOwnerExit activates a termination handoff which was
// deliberately hidden from the settlement wake while an older worker still
// owned the gate. The exact epoch/token means an old exit can neither activate
// nor clear a later gate acquired in the same lifecycle.
func (m *Biz) settleRuntimeDriveGateAtOwnerExit(ctx context.Context, request runtimeDriveGateSettlementRequest) bool {
	if m == nil || request.DriveKey == "" || request.GateEpoch == 0 || request.GateToken == 0 {
		return false
	}
	m.driveMu.Lock()
	if current, ok := m.runtimeDriveGateSettlements[request.DriveKey]; ok &&
		current.Epoch == request.GateEpoch && current.GateToken == request.GateToken && current.Request.AwaitGateOwnerExit {
		pending := current.Request
		pending.AwaitGateOwnerExit = false
		request = mergeRuntimeDriveGateSettlementRequest(request, pending)
		request.AwaitGateOwnerExit = false
		current.Request = request
		m.runtimeDriveGateSettlements[request.DriveKey] = current
	}
	m.driveMu.Unlock()
	return m.settleRuntimeDriveGateOrRetain(ctx, request)
}

func (m *Biz) activateRuntimeDriveGateSettlementAtOwnerExit(request runtimeDriveGateSettlementRequest) {
	if m == nil || request.DriveKey == "" || request.GateEpoch == 0 || request.GateToken == 0 {
		return
	}
	m.driveMu.Lock()
	if current, ok := m.runtimeDriveGateSettlements[request.DriveKey]; ok &&
		current.Epoch == request.GateEpoch && current.GateToken == request.GateToken && current.Request.AwaitGateOwnerExit {
		current.Request.AwaitGateOwnerExit = false
		m.runtimeDriveGateSettlements[request.DriveKey] = current
		m.armRuntimeDriveGateSettlementWakeLocked(runtimeDriveGateSettlementWakeDelay)
	}
	m.driveMu.Unlock()
}

func (m *Biz) currentRuntimeDriveGate(key string) (runtimeDriveGateRegistration, bool) {
	if m == nil || key == "" {
		return runtimeDriveGateRegistration{}, false
	}
	m.driveMu.Lock()
	gate, ok := m.drivingRuns[key]
	m.driveMu.Unlock()
	return gate, ok
}

func (m *Biz) runtimeDriveGateSettlementIdentity(key string) (uint64, uint64) {
	if m == nil || key == "" {
		return 0, 0
	}
	m.driveMu.Lock()
	registration, ok := m.runtimeDriveGateSettlements[key]
	m.driveMu.Unlock()
	if !ok {
		return 0, 0
	}
	return registration.Epoch, registration.Token
}

func (m *Biz) clearRuntimeDriveGateSettlement(key string, epoch uint64, token uint64) {
	if m == nil || key == "" {
		return
	}
	m.driveMu.Lock()
	if current, ok := m.runtimeDriveGateSettlements[key]; ok &&
		(epoch == 0 || (current.Epoch == epoch && current.Token == token)) {
		delete(m.runtimeDriveGateSettlements, key)
	}
	m.driveMu.Unlock()
}

func (m *Biz) reconcilePendingRuntimeDriveGateSettlements() {
	if m == nil {
		return
	}
	m.driveMu.Lock()
	if m.runtimeDriveGateSettlementRunning || m.runtimeLeaseTimerClosing {
		m.driveMu.Unlock()
		return
	}
	m.runtimeDriveGateSettlementRunning = true
	registrations := make(map[string]runtimeDriveGateSettlementRegistration, len(m.runtimeDriveGateSettlements))
	for key, registration := range m.runtimeDriveGateSettlements {
		registrations[key] = registration
	}
	m.driveMu.Unlock()
	defer func() {
		m.driveMu.Lock()
		m.runtimeDriveGateSettlementRunning = false
		m.driveMu.Unlock()
	}()

	for key, registration := range registrations {
		m.driveMu.Lock()
		current, pending := m.runtimeDriveGateSettlements[key]
		gate, ownsGate := m.drivingRuns[key]
		active := pending && current.Epoch == registration.Epoch && current.Token == registration.Token &&
			current.GateToken == registration.GateToken && ownsGate && gate.Epoch == registration.Epoch && gate.Token == registration.GateToken &&
			registration.Epoch == m.runtimeLeaseTimerEpoch && !m.runtimeLeaseTimerClosing
		m.driveMu.Unlock()
		if !active {
			m.clearRuntimeDriveGateSettlement(key, registration.Epoch, registration.Token)
			continue
		}
		if registration.Request.AwaitGateOwnerExit {
			continue
		}
		m.lifecycleMu.RLock()
		if m.runtimeController != nil {
			m.settleRuntimeDriveGateOrRetain(m.runtimeLifecycleContext(), registration.Request)
		}
		m.lifecycleMu.RUnlock()
	}
}

func (m *Biz) enqueueRuntimeDriveLeaseTimer(ctx context.Context, lease runtimecontract.RuntimeDriveLease, source string) bool {
	if m == nil || m.BackgroundRoutine() == nil || lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return false
	}
	if err := m.validateWorkspaceRetirementLeaseTimerAdmission(ctx, lease); err != nil {
		return false
	}
	key := runtimeDriveKey(lease.WorkspaceID, lease.RunID)
	registered, accepted, epoch, gateToken := m.beginRuntimeLeaseTimer(key, lease)
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
			m.endRuntimeLeaseTimerExact(key, lease, epoch, gateToken)
			m.retainPendingRuntimeDriveLeaseMount(key, lease)
			m.retainRuntimeDriveGateSettlement(runtimeDriveGateSettlementRequest{
				WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, DriveKey: key,
				GateEpoch: epoch, GateToken: gateToken, Source: source + ":timer_submission_rejected",
				LeaseHint: lease.Clone(), HasLeaseHint: true,
			})
		}
	}()
	if !m.launchRuntimeDriveLeaseTimer(ctx, lease, source, epoch, gateToken) {
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
func (m *Biz) launchRuntimeDriveLeaseTimer(_ context.Context, lease runtimecontract.RuntimeDriveLease, source string, epoch uint64, gateToken uint64) bool {
	if m == nil || m.BackgroundRoutine() == nil || lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return false
	}
	key := runtimeDriveKey(lease.WorkspaceID, lease.RunID)
	// Context publication is fenced by Stop's closing+epoch transition. Never
	// hold driveMu (or another Stop fence) across BackgroundRoutine submission:
	// a bounded task queue is allowed to block its caller.
	m.driveMu.Lock()
	registration, registered := m.retryLeaseTimers[key]
	gate, ownsGate := m.drivingRuns[key]
	if m.runtimeLeaseTimerClosing || epoch == 0 || gateToken == 0 || epoch != m.runtimeLeaseTimerEpoch ||
		!registered || registration.Epoch != epoch || registration.GateToken != gateToken ||
		!runtimecontract.SameRuntimeDriveLeaseIdentity(registration.Lease, lease) ||
		!ownsGate || gate.Epoch != epoch || gate.Token != gateToken {
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
	err := m.scheduleOwnerWork(m.BackgroundRoutine().AsyncFunction, func() {
		gateTransferred := false
		stopCanceled := false
		var authoritativeSuccessor *runtimecontract.RuntimeDriveLease
		defer func() {
			if gateTransferred {
				m.endRuntimeLeaseTimerExact(key, lease, epoch, gateToken)
				return
			}
			if stopCanceled {
				m.releaseCanceledRuntimeDriveLeaseTimerExact(key, lease, epoch, gateToken)
				return
			}
			m.finishRuntimeDriveLeaseTimerAtGateExit(retryCtx, lease, authoritativeSuccessor, source, epoch, gateToken)
		}()
		if !m.runtimeDriveLeaseTimerCallbackActive(retryCtx, key, lease, epoch, gateToken) {
			stopCanceled = m.runtimeDriveLeaseTimerLifecycleCanceled(retryCtx, epoch)
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
		if !m.runtimeDriveLeaseTimerCallbackActive(retryCtx, key, lease, epoch, gateToken) {
			stopCanceled = m.runtimeDriveLeaseTimerLifecycleCanceled(retryCtx, epoch)
			return
		}
		m.observeRuntimeDrive(retryCtx, orchestratorevents.RuntimeDriveObservedData{
			WorkspaceID: lease.WorkspaceID, RunID: lease.RunID, Kind: orchestratorevents.RuntimeDriveObservationRetryTimerFired,
			AggregateRevision: lease.AggregateRevision, RecoveryGeneration: lease.RecoveryGeneration, FailureCategory: string(lease.Reason), LeaseState: lease.State, Source: source,
		})
		claimedLease, claimed, claimErr := m.claimRuntimeDriveLeaseWhileStarted(retryCtx, lease, time.Now().UTC().Format(time.RFC3339Nano), epoch, gateToken)
		if claimErr != nil {
			if errors.Is(claimErr, context.Canceled) && m.runtimeDriveLeaseTimerLifecycleCanceled(retryCtx, epoch) {
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
		if err := m.scheduleRuntimeDriveWithLease(retryCtx, claimedLease.WorkspaceID, claimedLease.RunID, "recovery_lease:"+source, claimedLease.RecoveryGeneration, epoch, gateToken); err != nil {
			if errors.Is(err, context.Canceled) && m.runtimeDriveLeaseTimerLifecycleCanceled(retryCtx, epoch) {
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

func (m *Biz) runtimeDriveLeaseTimerCallbackActive(ctx context.Context, key string, lease runtimecontract.RuntimeDriveLease, epoch uint64, gateToken uint64) bool {
	if m == nil || ctx == nil || key == "" || epoch == 0 || gateToken == 0 || ctx.Err() != nil {
		return false
	}
	m.driveMu.Lock()
	registration, registered := m.retryLeaseTimers[key]
	gate, ownsGate := m.drivingRuns[key]
	active := !m.runtimeLeaseTimerClosing && m.runtimeLeaseTimerEpoch == epoch && registered &&
		registration.Epoch == epoch && registration.GateToken == gateToken &&
		runtimecontract.SameRuntimeDriveLeaseIdentity(registration.Lease, lease) &&
		ownsGate && gate.Epoch == epoch && gate.Token == gateToken
	if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" && (runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != "") {
		active = false
	}
	m.driveMu.Unlock()
	return active && ctx.Err() == nil
}

func (m *Biz) runtimeDriveLeaseTimerLifecycleCanceled(ctx context.Context, epoch uint64) bool {
	if m == nil || ctx == nil || epoch == 0 {
		return true
	}
	m.driveMu.Lock()
	canceled := ctx.Err() != nil || m.runtimeLeaseTimerClosing || m.runtimeLeaseTimerEpoch != epoch
	m.driveMu.Unlock()
	return canceled
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
	gateToken uint64,
) {
	key := runtimeDriveKey(retiring.WorkspaceID, retiring.RunID)
	m.endRuntimeLeaseTimerExact(key, retiring, epoch, gateToken)
	settlement := runtimeDriveGateSettlementRequest{
		WorkspaceID: retiring.WorkspaceID, RunID: retiring.RunID, DriveKey: key,
		GateEpoch: epoch, GateToken: gateToken, Source: source + ":timer_exit",
		LeaseHint: retiring.Clone(), HasLeaseHint: true, ReleaseWhenNoLease: false,
	}
	if authoritativeSuccessor != nil && authoritativeSuccessor.Validate() == nil && authoritativeSuccessor.State == runtimecontract.RuntimeDriveLeaseScheduled {
		settlement.LeaseHint = authoritativeSuccessor.Clone()
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	if m.runtimeController == nil || !m.settleRuntimeDriveGateAtOwnerExit(ctx, settlement) {
		slog.Warn("workorch Runtime lease timer exit retained owner gate for lifecycle reconciliation", "workspaceID", retiring.WorkspaceID, "runID", retiring.RunID, "generation", retiring.RecoveryGeneration, "source", source)
	}
}

func (m *Biz) releaseCanceledRuntimeDriveLeaseTimerExact(key string, lease runtimecontract.RuntimeDriveLease, epoch uint64, gateToken uint64) {
	if m == nil || key == "" || lease.RecoveryGeneration == 0 || epoch == 0 || gateToken == 0 {
		return
	}
	m.driveMu.Lock()
	registration, registered := m.retryLeaseTimers[key]
	exactRegistration := registered && registration.Epoch == epoch && registration.GateToken == gateToken &&
		runtimecontract.SameRuntimeDriveLeaseIdentity(registration.Lease, lease)
	if exactRegistration {
		delete(m.retryLeaseTimers, key)
	}
	// Only lifecycle cancellation of the exact still-current registration may
	// discard its gate. Identity supersession (for example G replaced by G+1 on
	// the same gate) is not cancellation authority for the newer registration.
	lifecycleCanceled := m.runtimeLeaseTimerClosing || m.runtimeLeaseTimerEpoch != epoch
	if gate, ok := m.drivingRuns[key]; lifecycleCanceled && exactRegistration && ok && gate.Epoch == epoch && gate.Token == gateToken {
		delete(m.persistedLeaseMounts, key)
		delete(m.runtimeDriveGateSettlements, key)
		delete(m.drivingRuns, key)
	}
	m.driveMu.Unlock()
}

// claimRuntimeDriveLeaseWhileStarted fences the timer's only Runtime owner
// dereference against Stop. The lock is released before handing a claimed
// generation to scheduleRuntimeDriveWithLease, whose worker acquires its own
// lifecycle read lock.
func (m *Biz) claimRuntimeDriveLeaseWhileStarted(ctx context.Context, lease runtimecontract.RuntimeDriveLease, now string, epoch uint64, gateToken uint64) (runtimecontract.RuntimeDriveLease, bool, error) {
	if m == nil {
		return runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	m.lifecycleMu.RLock()
	defer m.lifecycleMu.RUnlock()
	if !m.runtimeDriveLeaseTimerCallbackActive(ctx, runtimeDriveKey(lease.WorkspaceID, lease.RunID), lease, epoch, gateToken) || m.runtimeController == nil {
		return runtimecontract.RuntimeDriveLease{}, false, context.Canceled
	}
	return m.runtimeClaimDriveLease(ctx, lease, now)
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
	gate, gateOwned := m.drivingRuns[key]
	registration, registered := m.retryLeaseTimers[key]
	pendingMount, mountPending := m.persistedLeaseMounts[key]
	pendingRegistration := gateOwned && registered && mountPending && registration.Epoch == epoch && registration.GateToken == gate.Token &&
		runtimecontract.SameRuntimeDriveLeaseIdentity(registration.Lease, pendingMount)
	if gateOwned && registered && registration.Epoch == epoch && registration.GateToken == gate.Token &&
		runtimecontract.SameRuntimeDriveLeaseIdentity(registration.Lease, lease) && !pendingRegistration {
		return false
	}
	if gateOwned && registered && registration.Epoch == epoch && registration.GateToken == gate.Token &&
		registration.Generation >= lease.RecoveryGeneration && !runtimecontract.SameRuntimeDriveLeaseIdentity(registration.Lease, lease) {
		return false
	}
	if m.drivingRuns == nil {
		m.drivingRuns = map[string]runtimeDriveGateRegistration{}
	}
	if !gateOwned {
		m.runtimeDriveGateToken++
		if m.runtimeDriveGateToken == 0 {
			m.runtimeDriveGateToken++
		}
		m.drivingRuns[key] = runtimeDriveGateRegistration{Epoch: epoch, Token: m.runtimeDriveGateToken}
		return true
	}
	if m.persistedLeaseMounts == nil {
		m.persistedLeaseMounts = map[string]runtimecontract.RuntimeDriveLease{}
	}
	current, exists := m.persistedLeaseMounts[key]
	if exists && (current.RecoveryGeneration > lease.RecoveryGeneration ||
		(current.RecoveryGeneration == lease.RecoveryGeneration && !runtimecontract.SameRuntimeDriveLeaseIdentity(current, lease))) {
		return false
	}
	if !exists || lease.RecoveryGeneration >= current.RecoveryGeneration {
		m.persistedLeaseMounts[key] = lease.Clone()
	}
	if pendingRegistration {
		// No callback owns a paired registration+latch; this caller may retry the
		// mount while retaining the same run gate. A live timer never has that
		// latch, so it still returns false above.
		return true
	}
	return false
}

// takePersistedRuntimeDriveLeaseMount is called only by the current gate
// owner. It either retains that gate for the exact persisted lease or releases
// the gate in the same critical section, so a concurrent handoff cannot land
// between the final check and gate release.
func (m *Biz) takePersistedRuntimeDriveLeaseMount(key string) (runtimecontract.RuntimeDriveLease, bool) {
	gate, ok := m.currentRuntimeDriveGate(key)
	if !ok {
		return runtimecontract.RuntimeDriveLease{}, false
	}
	lease, retained, _ := m.takePersistedRuntimeDriveLeaseMountExact(key, gate.Epoch, gate.Token, true)
	return lease, retained
}

func (m *Biz) takePersistedRuntimeDriveLeaseMountExact(key string, epoch uint64, gateToken uint64, releaseIfAbsent bool) (runtimecontract.RuntimeDriveLease, bool, bool) {
	if m == nil || key == "" {
		return runtimecontract.RuntimeDriveLease{}, false, false
	}
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	gate, ownsGate := m.drivingRuns[key]
	if !ownsGate || gate.Epoch != epoch || gate.Token != gateToken {
		return runtimecontract.RuntimeDriveLease{}, false, false
	}
	if lease, ok := m.persistedLeaseMounts[key]; ok {
		delete(m.persistedLeaseMounts, key)
		if registration, registered := m.retryLeaseTimers[key]; registered &&
			registration.Generation == lease.RecoveryGeneration && registration.Epoch == epoch && registration.GateToken == gateToken {
			delete(m.retryLeaseTimers, key)
		}
		return lease.Clone(), true, false
	}
	if settlement, ok := m.runtimeDriveGateSettlements[key]; ok &&
		settlement.Epoch == epoch && settlement.GateToken == gateToken {
		// Gate release and handoff registration share this critical section.
		// If the old owner is leaving, activate the exact dormant settlement;
		// an already-active settlement simply keeps ownership. In either case
		// the gate cannot disappear between a fresh no-lease decision and the
		// final local release.
		if settlement.Request.AwaitGateOwnerExit {
			settlement.Request.AwaitGateOwnerExit = false
			m.runtimeDriveGateSettlements[key] = settlement
		}
		m.armRuntimeDriveGateSettlementWakeLocked(runtimeDriveGateSettlementWakeDelay)
		return runtimecontract.RuntimeDriveLease{}, false, false
	}
	if releaseIfAbsent {
		delete(m.drivingRuns, key)
		return runtimecontract.RuntimeDriveLease{}, false, true
	}
	return runtimecontract.RuntimeDriveLease{}, false, false
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

func (m *Biz) beginRuntimeDrive(key string) bool {
	_, accepted := m.beginRuntimeDriveWithToken(key)
	return accepted
}

func (m *Biz) beginRuntimeDriveWithToken(key string) (uint64, bool) {
	if m == nil || key == "" {
		return 0, false
	}
	m.runtimeMutationFence.RLock()
	defer m.runtimeMutationFence.RUnlock()
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.runtimeLeaseTimerClosing {
		return 0, false
	}
	if workspaceID := runtimeDriveWorkspaceID(key); workspaceID == "" || runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != "" {
		return 0, false
	}
	if m.runtimeLeaseTimerEpoch == 0 {
		m.runtimeLeaseTimerEpoch = 1
	}
	if m.drivingRuns == nil {
		m.drivingRuns = map[string]runtimeDriveGateRegistration{}
	}
	if _, ok := m.drivingRuns[key]; ok {
		return 0, false
	}
	m.runtimeDriveGateToken++
	if m.runtimeDriveGateToken == 0 {
		m.runtimeDriveGateToken++
	}
	token := m.runtimeDriveGateToken
	m.drivingRuns[key] = runtimeDriveGateRegistration{Epoch: m.runtimeLeaseTimerEpoch, Token: token}
	return token, true
}

func (m *Biz) endRuntimeDriveExact(key string, epoch uint64, gateToken uint64) {
	if m == nil || key == "" || epoch == 0 || gateToken == 0 {
		return
	}
	m.driveMu.Lock()
	if current, ok := m.drivingRuns[key]; ok && current.Epoch == epoch && current.Token == gateToken {
		delete(m.drivingRuns, key)
	}
	m.driveMu.Unlock()
}

func (m *Biz) beginRuntimeLeaseTimer(key string, lease runtimecontract.RuntimeDriveLease) (registered bool, accepted bool, epoch uint64, gateToken uint64) {
	generation := lease.RecoveryGeneration
	if m == nil || key == "" || generation == 0 || lease.Validate() != nil || lease.State != runtimecontract.RuntimeDriveLeaseScheduled || key != runtimeDriveKey(lease.WorkspaceID, lease.RunID) {
		return false, false, 0, 0
	}
	m.runtimeMutationFence.RLock()
	defer m.runtimeMutationFence.RUnlock()
	m.driveMu.Lock()
	defer m.driveMu.Unlock()
	if m.runtimeLeaseTimerClosing {
		return false, false, m.runtimeLeaseTimerEpoch, 0
	}
	if workspaceID := runtimeDriveWorkspaceID(key); workspaceID == "" || runWorkspaceRetirementFenceIsRetired(m.retiredWorkspaces[workspaceID]) || m.quiescingWorkspaces[workspaceID].WorkspaceID != "" {
		return false, false, m.runtimeLeaseTimerEpoch, 0
	}
	if m.runtimeLeaseTimerEpoch == 0 {
		m.runtimeLeaseTimerEpoch = 1
	}
	epoch = m.runtimeLeaseTimerEpoch
	gate, ownsGate := m.drivingRuns[key]
	if !ownsGate || gate.Epoch != epoch || gate.Token == 0 {
		return false, false, epoch, 0
	}
	gateToken = gate.Token
	if m.retryLeaseTimers == nil {
		m.retryLeaseTimers = map[string]runtimeLeaseTimerRegistration{}
	}
	if current, ok := m.retryLeaseTimers[key]; ok && current.Generation == generation && current.Epoch == epoch && current.GateToken == gate.Token {
		if !runtimecontract.SameRuntimeDriveLeaseIdentity(current.Lease, lease) {
			return false, false, epoch, gateToken
		}
		if pending, exists := m.persistedLeaseMounts[key]; exists && pending.RecoveryGeneration == generation {
			delete(m.persistedLeaseMounts, key)
			return true, true, epoch, gateToken
		}
		if pending, exists := m.persistedLeaseMounts[key]; exists && pending.RecoveryGeneration != generation {
			return false, false, epoch, gateToken
		}
		return false, true, epoch, gateToken
	}
	if current, ok := m.retryLeaseTimers[key]; ok && current.Epoch == epoch && current.GateToken == gate.Token && current.Generation > generation {
		return false, false, epoch, gateToken
	}
	m.retryLeaseTimers[key] = runtimeLeaseTimerRegistration{Generation: generation, Epoch: epoch, GateToken: gate.Token, Lease: lease.Clone()}
	return true, true, epoch, gateToken
}

func (m *Biz) endRuntimeLeaseTimerExact(key string, lease runtimecontract.RuntimeDriveLease, epoch uint64, gateToken uint64) {
	if m == nil || key == "" || lease.RecoveryGeneration == 0 || epoch == 0 || gateToken == 0 {
		return
	}
	m.driveMu.Lock()
	if current, ok := m.retryLeaseTimers[key]; ok && current.Epoch == epoch && current.GateToken == gateToken &&
		runtimecontract.SameRuntimeDriveLeaseIdentity(current.Lease, lease) {
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
	var ownerTransient *runtime.RuntimeOwnerTransientError
	var semanticRejection *runtime.RuntimeSemanticRejectionError
	var executionBudgetExhausted *runtimecontract.RuntimeBudgetExhaustedError
	var durableDispatchBudgetExhausted *runtimecontract.RuntimeDurableDispatchBudgetExhaustedError
	var timeout interface{ Timeout() bool }
	var temporary interface{ Temporary() bool }
	switch {
	case errors.As(err, &durableDispatchBudgetExhausted):
		return runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionBudgetExhausted, Code: "execution_budget_exhausted",
			Signature: "execution-budget-exhausted/durable-dispatch/" + string(durableDispatchBudgetExhausted.CoverageWorkItemID), Message: msg,
		}
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
	case errors.As(err, &ownerTransient):
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassOwnerTransientUnavailable, Code: "owner_transient", Signature: "owner-transient", Message: msg}
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
		// Generic timeout/Temporary interfaces do not prove which owner
		// operation failed or whether it is safe to replay. Only the typed Run
		// persistence outcome above may enter generic SM-4 recovery.
		return runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment,
			Code:  "runtime_owner_outcome_untyped", Signature: "runtime-owner-outcome-untyped",
			Message: "runtime owner retry requires a typed persistence outcome",
		}
	default:
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "runtime-drive-failed", Message: msg}
	}
}

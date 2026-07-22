package orchestrator

// Owner lifecycle and Runtime V2 authority implementation.

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
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
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
	bizPtr          *RuntimeOrchestrator
	evidenceRecords repository.EvidenceRecordRepository
	rawTopics       []string
	// lifecycleMu protects Runtime V2 owner dependencies while a background
	// drive is using them. Stop takes the write lock before clearing pointers;
	// every drive holds the read lock through authority and projection work.
	lifecycleMu sync.RWMutex
	driveMu     sync.Mutex
	drivingRuns map[string]struct{}
	started     bool
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
	m.bizPtr = NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runEventRepository{hub: m.EventHub()},
		workspaceEventRepository{hub: m.EventHub()},
		decisionEventTicketRepository{hub: m.EventHub()},
		decisionEventResolutionRepository{hub: m.EventHub()},
		approvalEventRepository{hub: m.EventHub()},
		RuntimeEvidenceRepositories{
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
		},
	)

	if err := m.bizPtr.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: m.EventHub()}); err != nil {
		return err
	}
	m.evidenceRecords = deps.EvidenceRecords
	return nil
}

func (m *Biz) Start() error {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	if len(m.rawTopics) == 0 {
		m.rawTopics = []string{
			orchestratorevents.TopicOrchestratorCommandStartRun,
			orchestratorevents.TopicOrchestratorCommandBlockDecision,
			orchestratorevents.TopicOrchestratorCommandBlockApproval,
			orchestratorevents.TopicOrchestratorCommandResumeRun,
			orchestratorevents.TopicOrchestratorCommandCancelRun,
			orchestratorevents.TopicOrchestratorCommandSupersedeRun,
			orchestratorevents.TopicOrchestratorCommandFailRun,
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
			orchestratorevents.TopicOrchestratorDriveRuntimeV2,
			orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
			orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt,
			orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution,
			orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
			decisionevents.TopicDecisionCommandRecordEvidence,
			approvalevents.TopicApprovalCommandRecordEvidence,
		)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorDriveRuntimeV2, m.handleRuntimeV2Drive)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, m.handleRuntimeV2OperationReceipt)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, m.handleRuntimeV2OperationReceipt)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution, m.handleRuntimeV2GovernanceResolution)
		m.SubscribeFunc(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, m.handleRuntimeV2GovernanceTicketResolution)
		m.SubscribeFunc(decisionevents.TopicDecisionCommandRecordEvidence, m.handleDecisionEvidenceCommand)
		m.SubscribeFunc(approvalevents.TopicApprovalCommandRecordEvidence, m.handleApprovalEvidenceCommand)
	}
	if m.started {
		return nil
	}
	// GMM-R1 / SM-4: recovery inventory must be acquired synchronously. A missing
	// Run owner subscriber is a startup failure, not a warning that loses recovery.
	if err := m.recoverRuntimeV2Startup(context.Background()); err != nil {
		return err
	}
	m.started = true
	return nil
}

func (m *Biz) recoverRuntimeV2Startup(ctx context.Context) error {
	if m == nil || m.bizPtr == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	items, err := queryRuntimeV2RecoveryInventory(ctx, m.EventHub())
	if err != nil {
		return fmt.Errorf("Runtime V2 startup recovery inventory: %w", err)
	}
	recovered := 0
	for _, item := range items {
		switch item.Class {
		case runevents.RuntimeV2RecoveryBlockedGovernance:
			if _, loadErr := m.bizPtr.LoadRuntimeV2(ctx, item.WorkspaceID, item.RunID); loadErr != nil {
				slog.Warn("workorch Runtime V2 blocked governance binding verification failed", "workspaceID", item.WorkspaceID, "runID", item.RunID, "err", loadErr)
			}
			continue
		case runevents.RuntimeV2RecoveryActiveNoDispatch, runevents.RuntimeV2RecoveryActiveSealedDispatch,
			runevents.RuntimeV2RecoveryTerminalDeliveryPending:
		default:
			continue
		}
		if err := m.scheduleRuntimeV2Drive(ctx, item.WorkspaceID, item.RunID, "startup_recovery:"+string(item.Class)); err != nil {
			slog.Warn("workorch Runtime V2 startup recovery schedule failed", "workspaceID", item.WorkspaceID, "runID", item.RunID, "class", item.Class, "err", err)
			continue
		}
		recovered++
	}
	slog.Info("workorch Runtime V2 startup recovery inventory complete", "recovered", recovered)
	return nil
}

func queryRuntimeV2RecoveryInventory(ctx context.Context, hub magicevent.Hub) ([]runevents.RuntimeV2RecoveryInventoryItem, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]runevents.RuntimeV2RecoveryInventoryItem, 0)
	cursor := ""
	for {
		result := hub.Send(magicevent.NewEventWithContext(
			runevents.TopicRunCommandRuntimeV2RecoveryInventory, ModuleID, runevents.CommandDestination,
			magicevent.NewHeader(), ctx, runevents.QueryRuntimeV2RecoveryInventoryCommand{Cursor: cursor, Limit: 100},
		))
		if result == nil {
			return nil, common.ErrInvalidArgument
		}
		data, magicErr := result.Get()
		if magicErr != nil {
			return nil, magicErr
		}
		page, ok := data.(runevents.QueryRuntimeV2RecoveryInventoryResult)
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
	m.rawTopics = nil
	m.bizPtr = nil
	m.evidenceRecords = nil
	m.started = false
	m.driveMu.Lock()
	m.drivingRuns = nil
	m.driveMu.Unlock()
	return nil
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
	if m == nil || m.bizPtr == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case orchestratorevents.TopicOrchestratorCommandStartRun:
		cmd, ok := ev.Data().(orchestratorevents.StartRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.handleStartRunCommand(ctx, common.RuntimeWorkspaceIDFromContext(ctx), cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandBlockDecision:
		cmd, ok := ev.Data().(orchestratorevents.BlockDecisionCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.bizPtr.BlockOnDecision(ctx, cmd.RunID, cmd.TicketID)
	case orchestratorevents.TopicOrchestratorCommandBlockApproval:
		cmd, ok := ev.Data().(orchestratorevents.BlockApprovalCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.bizPtr.BlockOnApproval(ctx, cmd.RunID, cmd.ApprovalID)
	case orchestratorevents.TopicOrchestratorCommandResumeRun:
		cmd, ok := ev.Data().(orchestratorevents.ResumeRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.resumeRunMagicCommand(ctx, cmd)
	case orchestratorevents.TopicOrchestratorCommandCancelRun:
		cmd, ok := ev.Data().(orchestratorevents.CancelRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.bizPtr.CancelRun(ctx, cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandSupersedeRun:
		cmd, ok := ev.Data().(orchestratorevents.SupersedeRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.bizPtr.SupersedeRun(ctx, cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandFailRun:
		cmd, ok := ev.Data().(orchestratorevents.FailRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.bizPtr.FailRun(ctx, cmd.RunID, cmd.Reason)
	case orchestratorevents.TopicOrchestratorCommandPurgeRun:
		cmd, ok := ev.Data().(orchestratorevents.PurgeRunCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		runResult, err := m.purgeRunOwnerResources(ctx, cmd.WorkspaceID, cmd.RunID)
		if err != nil {
			return orchestratorevents.OrchestratorCommandResult{}, err
		}
		deleted, err := purgeRunEvidence(ctx, m.bizPtr.EvidenceRepositories(), cmd.RunID)
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

func (m *Biz) handleStartRunCommand(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) error {
	if m == nil || m.bizPtr == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	if err := m.bizPtr.AcceptRunStart(ctx, runID); err != nil {
		return err
	}
	return m.scheduleRuntimeV2Drive(ctx, workspaceID, runID, "start")
}

func (m *Biz) resumeRunMagicCommand(ctx context.Context, cmd orchestratorevents.ResumeRunCommand) error {
	if m == nil || m.bizPtr == nil || cmd.WorkspaceID == "" || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	if err := m.bizPtr.CanResumeRun(ctx, cmd.RunID); err != nil {
		return err
	}
	if err := m.bizPtr.ActivateRuntimeV2Run(ctx, cmd.WorkspaceID, cmd.RunID); err != nil {
		return err
	}
	return m.scheduleRuntimeV2Drive(ctx, cmd.WorkspaceID, cmd.RunID, "resume")
}

func (m *Biz) scheduleRuntimeV2Drive(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, source string) error {
	if m == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	driveKey := string(workspaceID) + "\x00" + string(runID)
	if !m.beginRuntimeV2Drive(driveKey) {
		return nil
	}
	drive := func(driveCtx context.Context) {
		retryDelivery := false
		// The singleflight key belongs to the scheduled drive, including the
		// no-op path where Stop wins before this background task begins. It
		// must therefore be released before the lifecycle readiness check.
		defer func() {
			m.endRuntimeV2Drive(driveKey)
			if retryDelivery {
				m.scheduleRuntimeV2RecoveryRetry(driveCtx, workspaceID, runID)
			}
		}()
		// Keep the Runtime V2 authority and its EventHub-backed dependencies
		// alive until this entire drive/projection/recovery branch returns.
		// A task that starts after Stop observes nil and exits without effects.
		m.lifecycleMu.RLock()
		if m.bizPtr == nil || m.EventHub() == nil {
			m.lifecycleMu.RUnlock()
			return
		}
		defer m.lifecycleMu.RUnlock()
		if err := m.driveAndProjectRuntimeV2Locked(driveCtx, workspaceID, runID); err != nil && !errors.Is(err, context.Canceled) {
			var projectionErr *runtimeV2ProjectionError
			if errors.As(err, &projectionErr) {
				slog.Warn("workorch Runtime V2 projection failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", err)
				_, consumed, budgetErr := m.bizPtr.ConsumeRuntimeV2RecoveryBudget(driveCtx, workspaceID, runID, runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
				if budgetErr != nil {
					slog.Warn("workorch Runtime V2 terminal delivery budget commit failed", "workspaceID", workspaceID, "runID", runID, "err", budgetErr)
					return
				}
				retryDelivery = consumed
				return
			}
			slog.Warn("workorch Runtime V2 drive failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", err)
			// RVC-R13: CAS conflict / owner transient / delivery failures must not
			// terminal a business Run as runtime contract defect.
			failure := classifyRuntimeV2DriveFailure(err)
			if !runtimecontract.FailureClassIsBusinessTerminal(failure.Class) {
				slog.Warn("workorch Runtime V2 drive non-terminal failure", "source", source, "workspaceID", workspaceID, "runID", runID, "class", failure.Class, "err", err)
				if budgetKind, ok := runtimeV2RecoveryBudgetKind(failure.Class); ok {
					_, consumed, budgetErr := m.bizPtr.ConsumeRuntimeV2RecoveryBudget(driveCtx, workspaceID, runID, budgetKind)
					if budgetErr != nil {
						slog.Warn("workorch Runtime V2 recovery budget commit failed", "workspaceID", workspaceID, "runID", runID, "kind", budgetKind, "err", budgetErr)
						return
					}
					retryDelivery = consumed
					if !consumed && failure.Class == runtimecontract.FailureClassOwnerTransientUnavailable {
						if _, recordErr := m.bizPtr.RecordRuntimeV2FailureFact(driveCtx, workspaceID, runID, failure); recordErr != nil {
							slog.Warn("workorch Runtime V2 exhausted operation recovery fact commit failed", "workspaceID", workspaceID, "runID", runID, "err", recordErr)
						} else {
							retryDelivery = true
						}
					}
				}
				return
			}
			aggregate, recordErr := m.bizPtr.RecordRuntimeV2FailureFact(driveCtx, workspaceID, runID, failure)
			if recordErr != nil {
				slog.Warn("workorch Runtime V2 drive failure fact commit failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", recordErr)
				return
			}
			if aggregate.Decision.Action != runtimecontract.RuntimeActionFail || aggregate.Decision.Terminal == nil {
				slog.Warn("workorch Runtime V2 drive failure did not reduce to terminal", "source", source, "workspaceID", workspaceID, "runID", runID)
				return
			}
			if projectErr := m.deliverRuntimeV2Terminal(driveCtx, aggregate); projectErr != nil {
				slog.Warn("workorch Runtime V2 drive failure delivery failed", "source", source, "workspaceID", workspaceID, "runID", runID, "err", projectErr)
				_, consumed, budgetErr := m.bizPtr.ConsumeRuntimeV2RecoveryBudget(driveCtx, workspaceID, runID, runtimecontract.RuntimeRecoveryBudgetTerminalDelivery)
				if budgetErr != nil {
					slog.Warn("workorch Runtime V2 terminal delivery budget commit failed", "workspaceID", workspaceID, "runID", runID, "err", budgetErr)
					return
				}
				retryDelivery = consumed
			}
		}
	}
	bgCtx := orchestratorBackgroundContext(ctx, workspaceID)
	if m.BackgroundRoutine() == nil {
		drive(bgCtx)
		return nil
	}
	if err := m.BackgroundRoutine().AsyncFunction(func() { drive(bgCtx) }); err != nil {
		m.endRuntimeV2Drive(driveKey)
		return err
	}
	return nil
}

func (m *Biz) scheduleRuntimeV2RecoveryRetry(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) {
	if m == nil || m.BackgroundRoutine() == nil || workspaceID == "" || runID == "" {
		return
	}
	retryCtx := orchestratorBackgroundContext(ctx, workspaceID)
	if err := m.BackgroundRoutine().AsyncFunction(func() {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-retryCtx.Done():
			return
		case <-timer.C:
		}
		if err := m.scheduleRuntimeV2Drive(retryCtx, workspaceID, runID, "recovery_retry"); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch Runtime V2 delivery retry scheduling failed", "workspaceID", workspaceID, "runID", runID, "err", err)
		}
	}); err != nil {
		slog.Warn("workorch Runtime V2 delivery retry was not accepted", "workspaceID", workspaceID, "runID", runID, "err", err)
	}
}

func runtimeV2RecoveryBudgetKind(class runtimecontract.FailureClass) (runtimecontract.RuntimeRecoveryBudgetKind, bool) {
	switch runtimecontract.FailureClassDisposition(class) {
	case runtimecontract.RuntimeFailureDispositionProviderRetry:
		return runtimecontract.RuntimeRecoveryBudgetProviderTransport, true
	case runtimecontract.RuntimeFailureDispositionOperationRecovery:
		return runtimecontract.RuntimeRecoveryBudgetOperationRecovery, true
	case runtimecontract.RuntimeFailureDispositionCASReload:
		return runtimecontract.RuntimeRecoveryBudgetCASReload, true
	case runtimecontract.RuntimeFailureDispositionDeliveryRetry:
		return runtimecontract.RuntimeRecoveryBudgetTerminalDelivery, true
	default:
		return "", false
	}
}

func (m *Biz) beginRuntimeV2Drive(key string) bool {
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

func (m *Biz) endRuntimeV2Drive(key string) {
	if m == nil || key == "" {
		return
	}
	m.driveMu.Lock()
	delete(m.drivingRuns, key)
	m.driveMu.Unlock()
}

func orchestratorBackgroundContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	bgCtx := context.Background()
	if workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	} else if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	}

	return bgCtx
}

func (m *Biz) readEvidenceMagicCommand(ev magicevent.Event) (orchestratorevents.OrchestratorCommandResult, error) {
	if m == nil || m.bizPtr == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(orchestratorevents.ListRunEvidenceCommand)
	if !ok || cmd.RunID == "" {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	if cmd.WorkspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
	}
	evidence := m.bizPtr.EvidenceRepositories()
	switch ev.ID() {
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun:
		if evidence.ToolCalls == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ToolCalls.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ToolCalls: items}, err
	case orchestratorevents.TopicOrchestratorCommandListPatchesByRun:
		if evidence.Patches == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.Patches.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{Patches: items}, err
	case orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun:
		if evidence.ValidationResults == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ValidationResults.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ValidationResults: items}, err
	case orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun:
		if evidence.CoverageWorkItems == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.CoverageWorkItems.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{CoverageWorkItems: items}, err
	case orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
		if evidence.ArtifactContinuations == nil {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		items, err := evidence.ArtifactContinuations.ListByRun(ctx, cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{ArtifactContinuations: items}, err
	default:
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
}

func runtimeV2RunIsTerminal(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func classifyRuntimeV2DriveFailure(err error) runtimecontract.FailureFact {
	if err == nil {
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeInvariantDefect, Code: "runtime_v2_drive_failed", Signature: "runtime-v2-drive-failed", Message: "unknown drive failure"}
	}
	msg := err.Error()
	switch {
	case errors.Is(err, common.ErrConflict):
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassCASConflict, Code: "cas_conflict", Signature: "cas-conflict", Message: msg}
	case strings.Contains(msg, "outcome_unknown"):
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassOperationOutcomeUnknown, Code: "operation_outcome_unknown", Signature: "operation-outcome-unknown", Message: msg}
	case strings.Contains(msg, "owner") && (strings.Contains(msg, "unavailable") || strings.Contains(msg, "timeout") || strings.Contains(msg, "temporarily")):
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassOwnerTransientUnavailable, Code: "owner_transient", Signature: "owner-transient", Message: msg}
	case strings.Contains(msg, "projection"):
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassProjectionDeliveryFailure, Code: "projection_delivery_failure", Signature: "projection-delivery", Message: msg}
	default:
		return runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_v2_drive_failed", Signature: "runtime-v2-drive-failed", Message: msg}
	}
}

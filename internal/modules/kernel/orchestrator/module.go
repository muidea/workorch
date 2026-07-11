package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
)

const ModuleID = modulecommon.ModuleID

type Service = orchestratorservice.Service
type RuntimeAdapter = orchestratorbiz.RuntimeAdapter
type RuntimeAdapterRegistry = orchestratorbiz.RuntimeAdapterRegistry

func NewRuntimeAdapterRegistry() *RuntimeAdapterRegistry {
	return orchestratorbiz.NewRuntimeAdapterRegistry()
}

type Dependencies struct {
	Runtime               orchestratorbiz.RuntimeAdapter
	Steps                 repository.StepRepository
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	RuntimeCheckpoints    repository.RuntimeCheckpointRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	RuntimePolicy         orchestratorbiz.RuntimePolicy
	CommandHub            magicevent.Hub
	BackgroundRoutine     task.BackgroundRoutine
}

type Module struct {
	bizPtr            *orchestratorbiz.RuntimeOrchestrator
	servicePtr        *orchestratorservice.Module
	steps             repository.StepRepository
	evidenceRecords   repository.EvidenceRecordRepository
	rawEventHub       magicevent.Hub
	rawObserver       magicevent.SimpleObserver
	rawTopics         []string
	runtimeObserver   magicevent.SimpleObserver
	runtimeTopics     []string
	backgroundRoutine task.BackgroundRoutine

	recoveryMu       sync.Mutex
	recoveryInFlight map[string]struct{}
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Runtime == nil ||
		deps.Steps == nil ||
		deps.ToolCalls == nil ||
		deps.Patches == nil ||
		deps.ValidationResults == nil ||
		deps.RuntimeCheckpoints == nil ||
		deps.EvidenceRecords == nil ||
		deps.CoverageWorkItems == nil ||
		deps.ArtifactContinuations == nil ||
		deps.CommandHub == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = orchestratorbiz.NewRuntimeOrchestratorWithResolutionsAndEvidence(
		deps.Runtime,
		runEventRepository{hub: deps.CommandHub},
		deps.Steps,
		workspaceEventRepository{hub: deps.CommandHub},
		decisionEventTicketRepository{hub: deps.CommandHub},
		decisionEventResolutionRepository{hub: deps.CommandHub},
		approvalEventRepository{hub: deps.CommandHub},
		orchestratorbiz.RuntimeEvidenceRepositories{
			Results:               resultEventResultRepository{hub: deps.CommandHub},
			Artifacts:             resultEventArtifactRepository{hub: deps.CommandHub},
			ToolCalls:             deps.ToolCalls,
			Patches:               deps.Patches,
			ValidationResults:     deps.ValidationResults,
			RuntimeCheckpoints:    deps.RuntimeCheckpoints,
			EvidenceRecords:       deps.EvidenceRecords,
			CoverageWorkItems:     deps.CoverageWorkItems,
			ArtifactContinuations: deps.ArtifactContinuations,
			MemoryItems:           memoryEventRepository{hub: deps.CommandHub},
			MemoryRecallTraces:    memoryRecallTraceEventRepository{hub: deps.CommandHub},
		},
		deps.RuntimePolicy,
	)

	m.servicePtr = orchestratorservice.New(orchestratorservice.Dependencies{
		StartRun:         m.bizPtr.StartRun,
		DispatchNextStep: m.bizPtr.DispatchNextStep,
		BlockOnDecision:  m.bizPtr.BlockOnDecision,
		BlockOnApproval:  m.bizPtr.BlockOnApproval,
		ResumeRun:        m.bizPtr.ResumeRun,
		ResumeRunPatch:   m.bizPtr.ResumeRunWithExecutionContextPatch,
		ValidateRun:      m.bizPtr.ValidateRun,
		CompleteRun:      m.bizPtr.CompleteRun,
		CancelRun:        m.bizPtr.CancelRun,
		SupersedeRun:     m.bizPtr.SupersedeRun,
		FailRun:          m.bizPtr.FailRun,
	})
	m.bizPtr.EnableEventDrivenDispatch(deps.CommandHub)
	m.steps = deps.Steps
	m.evidenceRecords = deps.EvidenceRecords
	m.rawEventHub = deps.CommandHub
	m.backgroundRoutine = deps.BackgroundRoutine
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.rawEventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.rawObserver != nil || m.runtimeObserver != nil {
		return nil
	}
	if m.rawEventHub != nil {
		m.runtimeObserver = magicevent.NewSimpleObserverWithMatchID(ModuleID+"/runtime", runtimeevent.NotificationDestination, m.rawEventHub)
		m.runtimeTopics = []string{
			runtimeevent.TopicRuntimeBlockExecuteProgress,
			runtimeevent.TopicRuntimeBlockExecuteCompleted,
			runtimeevent.TopicRuntimeBlockExecuteFailed,
		}
		m.runtimeObserver.Subscribe(runtimeevent.TopicRuntimeBlockExecuteProgress, m.handleRuntimeProgressEvent)
		m.runtimeObserver.Subscribe(runtimeevent.TopicRuntimeBlockExecuteCompleted, m.handleRuntimeResultEvent)
		m.runtimeObserver.Subscribe(runtimeevent.TopicRuntimeBlockExecuteFailed, m.handleRuntimeResultEvent)
	}
	if m.rawEventHub != nil && m.rawObserver == nil {
		m.rawObserver = magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, m.rawEventHub)
		m.rawTopics = []string{
			orchestratorevents.TopicOrchestratorCommandStartRun,
			orchestratorevents.TopicOrchestratorCommandBlockDecision,
			orchestratorevents.TopicOrchestratorCommandBlockApproval,
			orchestratorevents.TopicOrchestratorCommandResumeRun,
			orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap,
			orchestratorevents.TopicOrchestratorCommandValidateRun,
			orchestratorevents.TopicOrchestratorCommandCompleteRun,
			orchestratorevents.TopicOrchestratorCommandCancelRun,
			orchestratorevents.TopicOrchestratorCommandSupersedeRun,
			orchestratorevents.TopicOrchestratorCommandFailRun,
			orchestratorevents.TopicOrchestratorCommandPurgeRun,
			orchestratorevents.TopicOrchestratorCommandUpdateRuntimePolicy,
			orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
			orchestratorevents.TopicOrchestratorCommandListPatchesByRun,
			orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun,
			orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
			orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun,
			orchestratorevents.TopicOrchestratorDispatchStepRequested,
		}
		for _, topic := range m.rawTopics {
			m.rawObserver.Subscribe(topic, m.handleMagicCommand)
		}
		m.rawTopics = append(m.rawTopics,
			decisionevents.TopicDecisionCommandRecordEvidence,
			approvalevents.TopicApprovalCommandRecordEvidence,
		)
		m.rawObserver.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, m.handleDecisionEvidenceCommand)
		m.rawObserver.Subscribe(approvalevents.TopicApprovalCommandRecordEvidence, m.handleApprovalEvidenceCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.runtimeObserver != nil {
		for idx := len(m.runtimeTopics) - 1; idx >= 0; idx-- {
			m.runtimeObserver.Unsubscribe(m.runtimeTopics[idx])
		}
	}
	m.runtimeObserver = nil
	m.runtimeTopics = nil
	if m.rawObserver != nil {
		for idx := len(m.rawTopics) - 1; idx >= 0; idx-- {
			m.rawObserver.Unsubscribe(m.rawTopics[idx])
		}
	}
	m.rawObserver = nil
	m.rawTopics = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.steps = nil
	m.evidenceRecords = nil
	m.rawEventHub = nil
	m.backgroundRoutine = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleMagicCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeMagicCommand(ev.Context(), ev.ID(), ev.Data())
	setOrchestratorCommandResult(result, response, err)
}

func (m *Module) executeMagicCommand(ctx context.Context, topic string, data any) (orchestratorevents.OrchestratorCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case orchestratorevents.TopicOrchestratorCommandStartRun:
		cmd, ok := data.(orchestratorevents.StartRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.handleStartRunCommand(ctx, common.RuntimeWorkspaceIDFromContext(ctx), cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandBlockDecision:
		cmd, ok := data.(orchestratorevents.BlockDecisionCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.BlockOnDecision(ctx, cmd.RunID, cmd.TicketID)
	case orchestratorevents.TopicOrchestratorCommandBlockApproval:
		cmd, ok := data.(orchestratorevents.BlockApprovalCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.BlockOnApproval(ctx, cmd.RunID, cmd.ApprovalID)
	case orchestratorevents.TopicOrchestratorCommandResumeRun:
		cmd, ok := data.(orchestratorevents.ResumeRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.resumeRunMagicCommand(ctx, cmd)
	case orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap:
		cmd, ok := data.(orchestratorevents.RecoverRuntimeGapCommand)
		if !ok {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.recoverRuntimeGapMagicCommand(ctx, cmd)
	case orchestratorevents.TopicOrchestratorCommandValidateRun:
		cmd, ok := data.(orchestratorevents.ValidateRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.ValidateRun(ctx, cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandCompleteRun:
		cmd, ok := data.(orchestratorevents.CompleteRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.CompleteRun(ctx, cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandCancelRun:
		cmd, ok := data.(orchestratorevents.CancelRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.CancelRun(ctx, cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandSupersedeRun:
		cmd, ok := data.(orchestratorevents.SupersedeRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.SupersedeRun(ctx, cmd.RunID)
	case orchestratorevents.TopicOrchestratorCommandFailRun:
		cmd, ok := data.(orchestratorevents.FailRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.servicePtr.FailRun(ctx, cmd.RunID, cmd.Reason)
	case orchestratorevents.TopicOrchestratorCommandPurgeRun:
		cmd, ok := data.(orchestratorevents.PurgeRunCommand)
		if !ok || cmd.RunID == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		_, err := purgeRunEvidence(ctx, m.steps, m.bizPtr.EvidenceRepositories(), cmd.RunID)
		return orchestratorevents.OrchestratorCommandResult{}, err
	case orchestratorevents.TopicOrchestratorCommandUpdateRuntimePolicy:
		cmd, ok := data.(orchestratorevents.UpdateRuntimePolicyCommand)
		if !ok {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.bizPtr.UpdateRuntimePolicy(cmd.Policy)
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
		orchestratorevents.TopicOrchestratorCommandListPatchesByRun,
		orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun,
		orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
		orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
		return m.readEvidenceMagicCommand(ctx, topic, data)
	case orchestratorevents.TopicOrchestratorDispatchStepRequested:
		cmd, ok := data.(orchestratorevents.DispatchStepCommand)
		if !ok || cmd.RunID == "" || cmd.StepID == "" || cmd.Kind == "" {
			return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
		}
		return orchestratorevents.OrchestratorCommandResult{}, m.dispatchRequestedStepCommand(ctx, cmd)
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

func (m *Module) handleStartRunCommand(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) error {
	if m == nil || m.bizPtr == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	if m.backgroundRoutine == nil {
		return m.servicePtr.StartRun(ctx, runID)
	}
	if err := m.bizPtr.AcceptRunStart(ctx, runID); err != nil {
		return err
	}
	bgCtx := orchestratorBackgroundContext(ctx, workspaceID)
	if err := m.backgroundRoutine.AsyncFunction(func() {
		if err := m.servicePtr.ResumeRun(bgCtx, runID); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Warn("workorch orchestrator background start dispatch failed", "workspaceID", workspaceID, "runID", runID, "err", err)
			_ = m.servicePtr.FailRun(bgCtx, runID, fmt.Sprintf("background start dispatch failed: %v", err))
		}
	}); err != nil {
		_ = m.servicePtr.FailRun(bgCtx, runID, fmt.Sprintf("background start dispatch scheduling failed: %v", err))
		return err
	}
	return nil
}

func (m *Module) resumeRunMagicCommand(ctx context.Context, cmd orchestratorevents.ResumeRunCommand) error {
	if m == nil || m.bizPtr == nil || m.servicePtr == nil || cmd.RunID == "" {
		return common.ErrInvalidArgument
	}
	patch := cloneCommandPatch(cmd.ExecutionContextPatch)
	if m.backgroundRoutine == nil {
		if len(patch) > 0 {
			return m.servicePtr.ResumeRunWithExecutionContextPatch(ctx, cmd.RunID, patch)
		}
		return m.servicePtr.ResumeRun(ctx, cmd.RunID)
	}
	if err := m.bizPtr.CanResumeRun(ctx, cmd.RunID); err != nil {
		return err
	}
	bgCtx := orchestratorBackgroundContext(ctx, cmd.WorkspaceID)
	return m.backgroundRoutine.AsyncFunction(func() {
		var err error
		if len(patch) > 0 {
			err = m.servicePtr.ResumeRunWithExecutionContextPatch(bgCtx, cmd.RunID, patch)
		} else {
			err = m.servicePtr.ResumeRun(bgCtx, cmd.RunID)
		}
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		slog.Warn("workorch orchestrator background resume failed", "workspaceID", cmd.WorkspaceID, "runID", cmd.RunID, "err", err)
	})
}

func (m *Module) recoverRuntimeGapMagicCommand(ctx context.Context, cmd orchestratorevents.RecoverRuntimeGapCommand) error {
	if m == nil || m.bizPtr == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	if cmd.RunID == "" || cmd.WorkspaceID == "" || cmd.StepID == "" || cmd.Gap == "" {
		return common.ErrInvalidArgument
	}
	if cmd.IdempotencyKey == "" {
		cmd.IdempotencyKey = runtimeGapRecoveryCommandIdempotencyKey(cmd)
	}
	if !recoverRuntimeGapCommandAllowed(cmd) {
		return common.ErrInvalidArgument
	}
	patch := runtimeGapRecoveryPatch(cmd)
	if m.backgroundRoutine == nil {
		return m.servicePtr.ResumeRunWithExecutionContextPatch(ctx, cmd.RunID, patch)
	}
	if err := m.bizPtr.CanResumeRun(ctx, cmd.RunID); err != nil {
		return err
	}
	if !m.markRuntimeGapRecoveryInFlight(cmd.IdempotencyKey) {
		return nil
	}
	bgCtx := orchestratorBackgroundContext(ctx, cmd.WorkspaceID)
	if err := m.backgroundRoutine.AsyncFunction(func() {
		defer m.clearRuntimeGapRecoveryInFlight(cmd.IdempotencyKey)
		err := m.servicePtr.ResumeRunWithExecutionContextPatch(bgCtx, cmd.RunID, patch)
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		slog.Warn("workorch orchestrator background runtime gap recovery failed", "workspaceID", cmd.WorkspaceID, "runID", cmd.RunID, "stepID", cmd.StepID, "gap", cmd.Gap, "err", err)
	}); err != nil {
		m.clearRuntimeGapRecoveryInFlight(cmd.IdempotencyKey)
		return err
	}
	return nil
}

func recoverRuntimeGapCommandAllowed(cmd runcontracts.RecoverRuntimeGapCommand) bool {
	switch strings.TrimSpace(cmd.Gap) {
	case "runtime_terminal_not_finalized":
		return strings.TrimSpace(cmd.RecoverySafety) == "terminal_result_evidence"
	case "worker_lost", "accepted_no_progress", "accepted_in_progress":
		return cmd.RecoveryPolicy.Normalized().AllowsWatchdogExternalRetry()
	default:
		return false
	}
}

func runtimeGapRecoveryPatch(cmd runcontracts.RecoverRuntimeGapCommand) map[string]any {
	patch := cloneCommandPatch(cmd.ExecutionContextPatch)
	if patch == nil {
		patch = map[string]any{}
	}
	recovery := map[string]any{
		"mode":        "runtime_gap_recovery",
		"nextAction":  "resume_run",
		"workspaceId": string(cmd.WorkspaceID),
		"runId":       string(cmd.RunID),
		"stepId":      string(cmd.StepID),
		"gap":         cmd.Gap,
	}
	if cmd.IdempotencyKey != "" {
		recovery["idempotencyKey"] = cmd.IdempotencyKey
	}
	if cmd.StepKind != "" {
		recovery["stepKind"] = cmd.StepKind
	}
	if cmd.RequestID != "" {
		recovery["requestId"] = cmd.RequestID
	}
	if cmd.QueueKey != "" {
		recovery["queueKey"] = cmd.QueueKey
	}
	if cmd.LatestStepTime != "" {
		recovery["latestStepTime"] = cmd.LatestStepTime
	}
	if policy := cmd.RecoveryPolicy.Normalized(); len(policy.ToMap()) > 0 {
		recovery["runtimeRecoveryPolicy"] = policy.ToMap()
	}
	if cmd.RecoverySafety != "" {
		recovery["recoverySafety"] = cmd.RecoverySafety
	}
	if cmd.Reason != "" {
		recovery["reason"] = cmd.Reason
	}
	patch["runtimeGapRecovery"] = recovery
	return patch
}

func runtimeGapRecoveryCommandIdempotencyKey(cmd runcontracts.RecoverRuntimeGapCommand) string {
	return "runtime-gap-recovery|" +
		recoveryKeyComponent(string(cmd.WorkspaceID)) + "|" +
		recoveryKeyComponent(string(cmd.RunID)) + "|" +
		recoveryKeyComponent(string(cmd.StepID)) + "|" +
		recoveryKeyComponent(cmd.Gap) + "|" +
		recoveryKeyComponent(cmd.RequestID) + "|" +
		recoveryKeyComponent(cmd.QueueKey)
}

func recoveryKeyComponent(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func (m *Module) markRuntimeGapRecoveryInFlight(key string) bool {
	if key == "" {
		return true
	}
	m.recoveryMu.Lock()
	defer m.recoveryMu.Unlock()
	if m.recoveryInFlight == nil {
		m.recoveryInFlight = map[string]struct{}{}
	}
	if _, ok := m.recoveryInFlight[key]; ok {
		return false
	}
	m.recoveryInFlight[key] = struct{}{}
	return true
}

func (m *Module) clearRuntimeGapRecoveryInFlight(key string) {
	if key == "" {
		return
	}
	m.recoveryMu.Lock()
	defer m.recoveryMu.Unlock()
	delete(m.recoveryInFlight, key)
}

func cloneCommandPatch(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func (m *Module) dispatchRequestedStepCommand(ctx context.Context, cmd orchestratorevents.DispatchStepCommand) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	if m.backgroundRoutine == nil {
		return m.bizPtr.DispatchRequestedStep(ctx, cmd)
	}
	bgCtx := orchestratorBackgroundContext(ctx, cmd.WorkspaceID)
	if err := m.backgroundRoutine.AsyncFunction(func() {
		if err := m.bizPtr.DispatchRequestedStep(bgCtx, cmd); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch orchestrator dispatch requested step failed",
				"workspaceID", cmd.WorkspaceID,
				"runID", cmd.RunID,
				"stepID", cmd.StepID,
				"err", err,
			)
		}
	}); err != nil {
		return err
	}
	return nil
}

func orchestratorBackgroundContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	bgCtx := context.Background()
	if workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	} else if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	}
	if providerID := common.RuntimeLLMProviderIDFromContext(ctx); providerID != "" {
		bgCtx = common.ContextWithRuntimeLLMProviderID(bgCtx, providerID)
	}
	return bgCtx
}

func (m *Module) handleRuntimeResultEvent(event magicevent.Event, result magicevent.Result) {
	setOrchestratorAck(result, m.handleRuntimeResultData(event.Context(), event.ID(), event.Data()))
}

func (m *Module) handleRuntimeResultData(ctx context.Context, topic string, data any) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	if m.backgroundRoutine == nil {
		return m.finalizeRuntimeResult(ctx, topic, data)
	}
	bgCtx := runtimeEventBackgroundContext(ctx, data)
	return m.backgroundRoutine.AsyncFunction(func() {
		if err := m.finalizeRuntimeResult(bgCtx, topic, data); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch orchestrator runtime result finalize failed",
				"topic", topic,
				"err", err,
			)
		}
	})
}

func (m *Module) handleRuntimeProgressEvent(event magicevent.Event, result magicevent.Result) {
	data, ok := event.Data().(runtimeevent.ProgressData)
	if !ok {
		setOrchestratorAck(result, common.ErrInvalidArgument)
		return
	}
	setOrchestratorAck(result, m.handleRuntimeProgressData(event.Context(), event.ID(), data))
}

func (m *Module) handleRuntimeProgressData(ctx context.Context, topic string, data runtimeevent.ProgressData) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	if m.backgroundRoutine == nil {
		return m.recordRuntimeProgress(ctx, data)
	}
	bgCtx := runtimeEventBackgroundContext(ctx, data)
	return m.backgroundRoutine.AsyncFunction(func() {
		if err := m.recordRuntimeProgress(bgCtx, data); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch orchestrator runtime progress record failed",
				"topic", topic,
				"err", err,
			)
		}
	})
}

func (m *Module) readEvidenceMagicCommand(ctx context.Context, topic string, data any) (orchestratorevents.OrchestratorCommandResult, error) {
	if m == nil || m.bizPtr == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := data.(orchestratorevents.ListRunEvidenceCommand)
	if !ok || cmd.RunID == "" {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	if cmd.WorkspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
	}
	evidence := m.bizPtr.EvidenceRepositories()
	switch topic {
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

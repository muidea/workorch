package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/muidea/magicCommon/task"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type Service = orchestratorservice.Service
type RuntimeAdapter = orchestratorbiz.RuntimeAdapter
type RuntimeAdapterRegistry = orchestratorbiz.RuntimeAdapterRegistry

func NewRuntimeAdapterRegistry() *RuntimeAdapterRegistry {
	return orchestratorbiz.NewRuntimeAdapterRegistry()
}

func NewPatchRuntimeAdapter() *orchestratorbiz.PatchRuntimeAdapter {
	return orchestratorbiz.NewPatchRuntimeAdapter()
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
	EventHub              events.Hub
	BackgroundRoutine     task.BackgroundRoutine
}

type Module struct {
	bizPtr            *orchestratorbiz.RuntimeOrchestrator
	servicePtr        *orchestratorservice.Module
	steps             repository.StepRepository
	eventHub          events.Hub
	backgroundRoutine task.BackgroundRoutine
	patchExecutor     *runtimeevent.Executor
	unsub             []func()

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
		deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = orchestratorbiz.NewRuntimeOrchestratorWithResolutionsAndEvidence(
		deps.Runtime,
		runEventRepository{hub: deps.EventHub},
		deps.Steps,
		workspaceEventRepository{hub: deps.EventHub},
		decisionEventTicketRepository{hub: deps.EventHub},
		decisionEventResolutionRepository{hub: deps.EventHub},
		approvalEventRepository{hub: deps.EventHub},
		orchestratorbiz.RuntimeEvidenceRepositories{
			Results:               resultEventResultRepository{hub: deps.EventHub},
			Artifacts:             resultEventArtifactRepository{hub: deps.EventHub},
			ToolCalls:             deps.ToolCalls,
			Patches:               deps.Patches,
			ValidationResults:     deps.ValidationResults,
			RuntimeCheckpoints:    deps.RuntimeCheckpoints,
			EvidenceRecords:       deps.EvidenceRecords,
			CoverageWorkItems:     deps.CoverageWorkItems,
			ArtifactContinuations: deps.ArtifactContinuations,
			MemoryItems:           memoryEventRepository{hub: deps.EventHub},
			MemoryRecallTraces:    memoryRecallTraceEventRepository{hub: deps.EventHub},
		},
		deps.EventHub,
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
	m.bizPtr.EnableEventDrivenDispatch()
	m.steps = deps.Steps
	m.eventHub = deps.EventHub
	m.backgroundRoutine = deps.BackgroundRoutine
	if deps.BackgroundRoutine != nil {
		m.patchExecutor = runtimeevent.NewExecutor(
			ModuleID,
			deps.EventHub,
			deps.BackgroundRoutine,
			orchestratorbiz.NewPatchRuntimeAdapter(),
			runtimemodel.RuntimeBlockKindPatchApply,
			runtimemodel.RuntimeBlockKindPatchPreview,
			runtimemodel.RuntimeBlockKindPatchRevert,
		)
	}
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if len(m.unsub) > 0 {
		return nil
	}
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandStartRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandBlockDecision, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandBlockApproval, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandValidateRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandCompleteRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandCancelRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandSupersedeRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandFailRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandPurgeRun, m.handlePurgeRunCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandUpdateRuntimePolicy, m.handleRuntimePolicyCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListToolCallsByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListPatchesByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorDispatchStepRequested, m.handleDispatchStepRequested),
		m.eventHub.Subscribe(runtimeevent.TopicRuntimeBlockExecuteProgress, m.handleRuntimeProgressEvent),
		m.eventHub.Subscribe(runtimeevent.TopicRuntimeBlockExecuteCompleted, m.handleRuntimeResultEvent),
		m.eventHub.Subscribe(runtimeevent.TopicRuntimeBlockExecuteFailed, m.handleRuntimeResultEvent),
	)
	if m.patchExecutor != nil {
		if err := m.patchExecutor.Run(); err != nil {
			for idx := len(m.unsub) - 1; idx >= 0; idx-- {
				if m.unsub[idx] != nil {
					m.unsub[idx]()
				}
			}
			m.unsub = nil
			return err
		}
	}
	return nil
}

func (m *Module) handleRuntimePolicyCommand(ctx context.Context, envelope events.Envelope) error {
	_ = ctx
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	cmd, ok := envelope.Payload[events.PayloadKeyCommand].(orchestratorcontracts.UpdateRuntimePolicyCommand)
	if !ok {
		return common.ErrInvalidArgument
	}
	if err := m.bizPtr.UpdateRuntimePolicy(cmd.Policy); err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
	})
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.patchExecutor != nil {
		m.patchExecutor.Teardown()
		m.patchExecutor = nil
	}
	for idx := len(m.unsub) - 1; idx >= 0; idx-- {
		if m.unsub[idx] != nil {
			m.unsub[idx]()
		}
	}
	m.unsub = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.steps = nil
	m.eventHub = nil
	m.backgroundRoutine = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	payload := envelope.Payload
	runID := common.RunID(stringPayload(payload, events.PayloadKeyRunID))
	if runID == "" {
		runID = common.RunID(envelope.RunID)
	}
	var err error
	switch envelope.Topic {
	case orchestratorevents.TopicOrchestratorCommandStartRun:
		err = m.handleStartRunCommand(ctx, envelope, runID)
	case orchestratorevents.TopicOrchestratorCommandBlockDecision:
		cmd, ok := payload[events.PayloadKeyCommand].(runcontracts.BlockOnDecisionCommand)
		if !ok {
			cmd = runcontracts.BlockOnDecisionCommand{RunID: runID, TicketID: common.TicketID(stringPayload(payload, events.PayloadKeyBlockingTicketID))}
		}
		if cmd.RunID == "" {
			cmd.RunID = runID
		}
		err = m.servicePtr.BlockOnDecision(ctx, cmd.RunID, cmd.TicketID)
	case orchestratorevents.TopicOrchestratorCommandBlockApproval:
		cmd, ok := payload[events.PayloadKeyCommand].(runcontracts.BlockOnApprovalCommand)
		if !ok {
			cmd = runcontracts.BlockOnApprovalCommand{RunID: runID, ApprovalID: common.ApprovalID(stringPayload(payload, events.PayloadKeyBlockingApprovalID))}
		}
		if cmd.RunID == "" {
			cmd.RunID = runID
		}
		err = m.servicePtr.BlockOnApproval(ctx, cmd.RunID, cmd.ApprovalID)
	case orchestratorevents.TopicOrchestratorCommandResumeRun:
		err = m.handleResumeRunCommand(ctx, envelope, runID)
	case orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap:
		err = m.handleRecoverRuntimeGapCommand(ctx, envelope, runID)
	case orchestratorevents.TopicOrchestratorCommandValidateRun:
		err = m.servicePtr.ValidateRun(ctx, runID)
	case orchestratorevents.TopicOrchestratorCommandCompleteRun:
		err = m.servicePtr.CompleteRun(ctx, runID)
	case orchestratorevents.TopicOrchestratorCommandCancelRun:
		err = m.servicePtr.CancelRun(ctx, runID)
	case orchestratorevents.TopicOrchestratorCommandSupersedeRun:
		err = m.servicePtr.SupersedeRun(ctx, runID)
	case orchestratorevents.TopicOrchestratorCommandFailRun:
		err = m.servicePtr.FailRun(ctx, runID, stringPayload(payload, events.PayloadKeyFailureReason))
	default:
		return common.ErrInvalidArgument
	}
	if err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        string(runID),
		StepID:       envelope.StepID,
	})
}

func (m *Module) handleStartRunCommand(ctx context.Context, envelope events.Envelope, runID common.RunID) error {
	if m == nil || m.bizPtr == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	if m.backgroundRoutine == nil {
		return m.servicePtr.StartRun(ctx, runID)
	}
	if err := m.bizPtr.AcceptRunStart(ctx, runID); err != nil {
		return err
	}
	bgCtx := orchestratorBackgroundContext(ctx, envelope)
	if err := m.backgroundRoutine.AsyncFunction(func() {
		if err := m.servicePtr.ResumeRun(bgCtx, runID); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Warn("workorch orchestrator background start dispatch failed", "workspaceID", envelope.WorkspaceID, "runID", runID, "err", err)
			_ = m.servicePtr.FailRun(bgCtx, runID, fmt.Sprintf("background start dispatch failed: %v", err))
		}
	}); err != nil {
		_ = m.servicePtr.FailRun(bgCtx, runID, fmt.Sprintf("background start dispatch scheduling failed: %v", err))
		return err
	}
	return nil
}

func (m *Module) handleResumeRunCommand(ctx context.Context, envelope events.Envelope, runID common.RunID) error {
	if m == nil || m.bizPtr == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	payload := envelope.Payload
	cmd, ok := payload[events.PayloadKeyCommand].(runcontracts.ResumeRunCommand)
	var patch map[string]any
	if ok {
		if cmd.RunID != "" {
			runID = cmd.RunID
		}
		patch = cloneCommandPatch(cmd.ExecutionContextPatch)
	} else {
		rawPatch, _ := payload[events.PayloadKeyExecutionContextPatch].(map[string]any)
		patch = cloneCommandPatch(rawPatch)
	}
	if runID == "" {
		return common.ErrInvalidArgument
	}
	if m.backgroundRoutine == nil {
		if len(patch) > 0 {
			return m.servicePtr.ResumeRunWithExecutionContextPatch(ctx, runID, patch)
		}
		return m.servicePtr.ResumeRun(ctx, runID)
	}
	if err := m.bizPtr.CanResumeRun(ctx, runID); err != nil {
		return err
	}
	bgCtx := orchestratorBackgroundContext(ctx, envelope)
	if err := m.backgroundRoutine.AsyncFunction(func() {
		var err error
		if len(patch) > 0 {
			err = m.servicePtr.ResumeRunWithExecutionContextPatch(bgCtx, runID, patch)
		} else {
			err = m.servicePtr.ResumeRun(bgCtx, runID)
		}
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		slog.Warn("workorch orchestrator background resume failed", "workspaceID", envelope.WorkspaceID, "runID", runID, "err", err)
	}); err != nil {
		return err
	}
	return nil
}

func (m *Module) handleRecoverRuntimeGapCommand(ctx context.Context, envelope events.Envelope, runID common.RunID) error {
	if m == nil || m.bizPtr == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	payload := envelope.Payload
	cmd, ok := payload[events.PayloadKeyCommand].(runcontracts.RecoverRuntimeGapCommand)
	if !ok {
		return common.ErrInvalidArgument
	}
	if cmd.RunID != "" {
		runID = cmd.RunID
	}
	if cmd.IdempotencyKey == "" {
		cmd.IdempotencyKey = stringPayload(payload, events.PayloadKeyIdempotencyKey)
	}
	if runID == "" || cmd.WorkspaceID == "" || cmd.StepID == "" || cmd.Gap == "" {
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
		return m.servicePtr.ResumeRunWithExecutionContextPatch(ctx, runID, patch)
	}
	if err := m.bizPtr.CanResumeRun(ctx, runID); err != nil {
		return err
	}
	if !m.markRuntimeGapRecoveryInFlight(cmd.IdempotencyKey) {
		return nil
	}
	bgCtx := orchestratorBackgroundContext(ctx, envelope)
	if err := m.backgroundRoutine.AsyncFunction(func() {
		defer m.clearRuntimeGapRecoveryInFlight(cmd.IdempotencyKey)
		err := m.servicePtr.ResumeRunWithExecutionContextPatch(bgCtx, runID, patch)
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		slog.Warn("workorch orchestrator background runtime gap recovery failed", "workspaceID", envelope.WorkspaceID, "runID", runID, "stepID", cmd.StepID, "gap", cmd.Gap, "err", err)
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

func (m *Module) handleDispatchStepRequested(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	cmd, err := dispatchStepCommandFromEnvelope(envelope)
	if err != nil {
		return err
	}
	if m.backgroundRoutine == nil {
		return m.bizPtr.DispatchRequestedStep(ctx, cmd)
	}
	bgCtx := orchestratorBackgroundContext(ctx, envelope)
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

func dispatchStepCommandFromEnvelope(envelope events.Envelope) (orchestratorcontracts.DispatchStepCommand, error) {
	if cmd, ok := envelope.Payload[events.PayloadKeyCommand].(orchestratorcontracts.DispatchStepCommand); ok {
		if cmd.WorkspaceID == "" {
			cmd.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
		}
		if cmd.RunID == "" {
			cmd.RunID = common.RunID(envelope.RunID)
		}
		if cmd.StepID == "" {
			cmd.StepID = common.StepID(envelope.StepID)
		}
		if cmd.IdempotencyKey == "" {
			cmd.IdempotencyKey = stringPayload(envelope.Payload, events.PayloadKeyIdempotencyKey)
		}
		if cmd.RunID == "" || cmd.StepID == "" || cmd.Kind == "" {
			return orchestratorcontracts.DispatchStepCommand{}, common.ErrInvalidArgument
		}
		return cmd, nil
	}
	cmd := orchestratorcontracts.DispatchStepCommand{
		WorkspaceID:    common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)),
		RunID:          common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)),
		StepID:         common.StepID(stringPayload(envelope.Payload, events.PayloadKeyStepID)),
		Kind:           stringPayload(envelope.Payload, events.PayloadKeyKind),
		IdempotencyKey: stringPayload(envelope.Payload, events.PayloadKeyIdempotencyKey),
	}
	if cmd.WorkspaceID == "" {
		cmd.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	if cmd.RunID == "" {
		cmd.RunID = common.RunID(envelope.RunID)
	}
	if cmd.StepID == "" {
		cmd.StepID = common.StepID(envelope.StepID)
	}
	if input, ok := envelope.Payload[events.PayloadKeyInput].(map[string]any); ok {
		cmd.Input = cloneCommandPatch(input)
	}
	if cmd.RunID == "" || cmd.StepID == "" || cmd.Kind == "" {
		return orchestratorcontracts.DispatchStepCommand{}, common.ErrInvalidArgument
	}
	return cmd, nil
}

func orchestratorBackgroundContext(ctx context.Context, envelope events.Envelope) context.Context {
	bgCtx := context.Background()
	if workspaceID := common.WorkspaceID(envelope.WorkspaceID); workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	} else if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	}
	if providerID := common.RuntimeLLMProviderIDFromContext(ctx); providerID != "" {
		bgCtx = common.ContextWithRuntimeLLMProviderID(bgCtx, providerID)
	}
	return bgCtx
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func (m *Module) handleRuntimeResultEvent(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	if m.backgroundRoutine == nil {
		return m.finalizeRuntimeResult(ctx, envelope)
	}
	bgCtx := orchestratorBackgroundContext(ctx, envelope)
	return m.backgroundRoutine.AsyncFunction(func() {
		if err := m.finalizeRuntimeResult(bgCtx, envelope); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch orchestrator runtime result finalize failed",
				"topic", envelope.Topic,
				"workspaceID", envelope.WorkspaceID,
				"runID", envelope.RunID,
				"stepID", envelope.StepID,
				"err", err,
			)
		}
	})
}

func (m *Module) handleRuntimeProgressEvent(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	if m.backgroundRoutine == nil {
		return m.recordRuntimeProgress(ctx, envelope)
	}
	bgCtx := orchestratorBackgroundContext(ctx, envelope)
	return m.backgroundRoutine.AsyncFunction(func() {
		if err := m.recordRuntimeProgress(bgCtx, envelope); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("workorch orchestrator runtime progress record failed",
				"topic", envelope.Topic,
				"workspaceID", envelope.WorkspaceID,
				"runID", envelope.RunID,
				"stepID", envelope.StepID,
				"err", err,
			)
		}
	})
}

func orchestratorScopedWorkspaceID(envelope events.Envelope) common.WorkspaceID {
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	if workspaceID == "" {
		workspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	return workspaceID
}

func (m *Module) handlePurgeRunCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	runID := purgeRunIDFromPayload(envelope.Payload)
	if runID == "" {
		runID = common.RunID(envelope.RunID)
	}
	if runID == "" {
		return common.ErrInvalidArgument
	}
	deleted, err := purgeRunEvidence(ctx, m.steps, m.bizPtr.EvidenceRepositories(), runID)
	if err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        string(runID),
		StepID:       envelope.StepID,
		Payload: map[string]any{
			events.PayloadKeyDeletedCount: deleted,
		},
	})
}

func purgeRunIDFromPayload(payload map[string]any) common.RunID {
	if payload == nil {
		return ""
	}
	if cmd, ok := payload[events.PayloadKeyCommand].(orchestratorcontracts.PurgeRunCommand); ok {
		return cmd.RunID
	}
	return common.RunID(stringPayload(payload, events.PayloadKeyRunID))
}

func (m *Module) handleEvidenceReadCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	if workspaceID := orchestratorScopedWorkspaceID(envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
	if runID == "" {
		runID = common.RunID(envelope.RunID)
	}
	if runID == "" {
		return common.ErrInvalidArgument
	}
	evidence := m.bizPtr.EvidenceRepositories()
	var items any
	var err error
	switch envelope.Topic {
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun:
		if evidence.ToolCalls == nil {
			return common.ErrInvalidArgument
		}
		items, err = evidence.ToolCalls.ListByRun(ctx, runID)
	case orchestratorevents.TopicOrchestratorCommandListPatchesByRun:
		if evidence.Patches == nil {
			return common.ErrInvalidArgument
		}
		items, err = evidence.Patches.ListByRun(ctx, runID)
	case orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun:
		if evidence.ValidationResults == nil {
			return common.ErrInvalidArgument
		}
		items, err = evidence.ValidationResults.ListByRun(ctx, runID)
	case orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun:
		if evidence.CoverageWorkItems == nil {
			return common.ErrInvalidArgument
		}
		items, err = evidence.CoverageWorkItems.ListByRun(ctx, runID)
	case orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
		if evidence.ArtifactContinuations == nil {
			return common.ErrInvalidArgument
		}
		items, err = evidence.ArtifactContinuations.ListByRun(ctx, runID)
	default:
		return common.ErrInvalidArgument
	}
	if err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        string(runID),
		StepID:       envelope.StepID,
		Payload: map[string]any{
			events.PayloadKeyItems: items,
		},
	})
}

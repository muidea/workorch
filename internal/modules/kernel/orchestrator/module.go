package orchestrator

import (
	"context"

	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
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
	ArtifactContinuations repository.ArtifactContinuationRepository
	RuntimePolicy         orchestratorbiz.RuntimePolicy
	EventHub              events.Hub
}

type Module struct {
	bizPtr     *orchestratorbiz.RuntimeOrchestrator
	servicePtr *orchestratorservice.Module
	steps      repository.StepRepository
	eventHub   events.Hub
	unsub      []func()
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
	m.steps = deps.Steps
	m.eventHub = deps.EventHub
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
		m.eventHub.Subscribe(events.TopicOrchestratorCommandStartRun, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandBlockDecision, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandBlockApproval, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandResumeRun, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandValidateRun, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandCompleteRun, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandCancelRun, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandSupersedeRun, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandFailRun, m.handleCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandPurgeRun, m.handlePurgeRunCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandListToolCallsByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandListPatchesByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandListValidationResultsByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(events.TopicOrchestratorCommandListArtifactContinuationsByRun, m.handleEvidenceReadCommand),
	)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
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
	case events.TopicOrchestratorCommandStartRun:
		err = m.servicePtr.StartRun(ctx, runID)
	case events.TopicOrchestratorCommandBlockDecision:
		err = m.servicePtr.BlockOnDecision(ctx, runID, common.TicketID(stringPayload(payload, events.PayloadKeyBlockingTicketID)))
	case events.TopicOrchestratorCommandBlockApproval:
		err = m.servicePtr.BlockOnApproval(ctx, runID, common.ApprovalID(stringPayload(payload, events.PayloadKeyBlockingApprovalID)))
	case events.TopicOrchestratorCommandResumeRun:
		if patch, _ := payload[events.PayloadKeyExecutionContextPatch].(map[string]any); len(patch) > 0 {
			err = m.servicePtr.ResumeRunWithExecutionContextPatch(ctx, runID, patch)
		} else {
			err = m.servicePtr.ResumeRun(ctx, runID)
		}
	case events.TopicOrchestratorCommandValidateRun:
		err = m.servicePtr.ValidateRun(ctx, runID)
	case events.TopicOrchestratorCommandCompleteRun:
		err = m.servicePtr.CompleteRun(ctx, runID)
	case events.TopicOrchestratorCommandCancelRun:
		err = m.servicePtr.CancelRun(ctx, runID)
	case events.TopicOrchestratorCommandSupersedeRun:
		err = m.servicePtr.SupersedeRun(ctx, runID)
	case events.TopicOrchestratorCommandFailRun:
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

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func (m *Module) handlePurgeRunCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
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

func (m *Module) handleEvidenceReadCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
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
	case events.TopicOrchestratorCommandListToolCallsByRun:
		if evidence.ToolCalls == nil {
			return common.ErrInvalidArgument
		}
		items, err = evidence.ToolCalls.ListByRun(ctx, runID)
	case events.TopicOrchestratorCommandListPatchesByRun:
		if evidence.Patches == nil {
			return common.ErrInvalidArgument
		}
		items, err = evidence.Patches.ListByRun(ctx, runID)
	case events.TopicOrchestratorCommandListValidationResultsByRun:
		if evidence.ValidationResults == nil {
			return common.ErrInvalidArgument
		}
		items, err = evidence.ValidationResults.ListByRun(ctx, runID)
	case events.TopicOrchestratorCommandListArtifactContinuationsByRun:
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

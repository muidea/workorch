package orchestrator

import (
	"context"

	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
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
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandStartRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandBlockDecision, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandBlockApproval, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandValidateRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandCompleteRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandCancelRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandSupersedeRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandFailRun, m.handleCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandPurgeRun, m.handlePurgeRunCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListToolCallsByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListPatchesByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun, m.handleEvidenceReadCommand),
		m.eventHub.Subscribe(orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun, m.handleEvidenceReadCommand),
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
	case orchestratorevents.TopicOrchestratorCommandStartRun:
		err = m.servicePtr.StartRun(ctx, runID)
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
		cmd, ok := payload[events.PayloadKeyCommand].(runcontracts.ResumeRunCommand)
		var patch map[string]any
		if ok {
			if cmd.RunID != "" {
				runID = cmd.RunID
			}
			patch = cmd.ExecutionContextPatch
		} else {
			patch, _ = payload[events.PayloadKeyExecutionContextPatch].(map[string]any)
		}
		if len(patch) > 0 {
			err = m.servicePtr.ResumeRunWithExecutionContextPatch(ctx, runID, patch)
		} else {
			err = m.servicePtr.ResumeRun(ctx, runID)
		}
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

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
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

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

type Dependencies struct {
	Runtime               orchestratorbiz.RuntimeAdapter
	Runs                  repository.RunRepository
	Steps                 repository.StepRepository
	Workspaces            repository.WorkspaceRepository
	Tickets               repository.DecisionTicketRepository
	Resolutions           repository.DecisionResolutionRepository
	Approvals             repository.ApprovalRepository
	Results               repository.ResultRepository
	Artifacts             repository.ArtifactRepository
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	RuntimeCheckpoints    repository.RuntimeCheckpointRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	MemoryItems           repository.MemoryItemRepository
	MemoryRecallTraces    repository.MemoryRecallTraceRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
	RuntimePolicy         orchestratorbiz.RuntimePolicy
	EventHub              events.Hub
}

type Module struct {
	bizPtr     *orchestratorbiz.RuntimeOrchestrator
	servicePtr *orchestratorservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Runtime == nil ||
		deps.Runs == nil ||
		deps.Steps == nil ||
		deps.Workspaces == nil ||
		deps.Tickets == nil ||
		deps.Resolutions == nil ||
		deps.Approvals == nil ||
		deps.Results == nil ||
		deps.Artifacts == nil ||
		deps.ToolCalls == nil ||
		deps.Patches == nil ||
		deps.ValidationResults == nil ||
		deps.RuntimeCheckpoints == nil ||
		deps.ArtifactContinuations == nil ||
		deps.MemoryItems == nil ||
		deps.MemoryRecallTraces == nil ||
		deps.AutoGovernanceTraces == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = orchestratorbiz.NewRuntimeOrchestratorWithResolutionsAndEvidence(
		deps.Runtime,
		deps.Runs,
		deps.Steps,
		deps.Workspaces,
		deps.Tickets,
		deps.Resolutions,
		deps.Approvals,
		orchestratorbiz.RuntimeEvidenceRepositories{
			Results:               deps.Results,
			Artifacts:             deps.Artifacts,
			ToolCalls:             deps.ToolCalls,
			Patches:               deps.Patches,
			ValidationResults:     deps.ValidationResults,
			RuntimeCheckpoints:    deps.RuntimeCheckpoints,
			ArtifactContinuations: deps.ArtifactContinuations,
			MemoryItems:           deps.MemoryItems,
			MemoryRecallTraces:    deps.MemoryRecallTraces,
			AutoGovernanceTraces:  deps.AutoGovernanceTraces,
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
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.servicePtr = nil
	m.bizPtr = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

package bootstrap

import (
	"context"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type Runtime struct {
	exports    Exports
	background magictask.BackgroundRoutine
}

type RepositoryExports struct {
	Souls                 repository.SoulRepository
	WorkspaceTemplates    repository.WorkspaceTemplateRepository
	MemoryItems           repository.MemoryItemRepository
	MemoryRecallTraces    repository.MemoryRecallTraceRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
	Skills                repository.SkillRepository
	MCPServers            repository.MCPServerRepository
	MCPCapabilities       repository.MCPCapabilityRepository
	Results               repository.ResultRepository
	Artifacts             repository.ArtifactRepository
	EventRecords          repository.EventRecordRepository
	Workspaces            repository.WorkspaceRepository
	Goals                 repository.GoalRepository
	DoDs                  repository.DoDRepository
	Runs                  repository.RunRepository
	Steps                 repository.StepRepository
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	RuntimeCheckpoints    repository.RuntimeCheckpointRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	Tickets               repository.DecisionTicketRepository
	Resolutions           repository.DecisionResolutionRepository
	Approvals             repository.ApprovalRepository
}

type InfrastructureExports struct {
	EventHub          events.Hub
	BackgroundRoutine magictask.BackgroundRoutine
	LoadConfig        func() (workorchconfig.Config, error)
}

type Exports struct {
	Repositories   RepositoryExports
	Infrastructure InfrastructureExports
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r.background != nil {
		r.background.Shutdown(ctx)
		r.background = nil
	}
	if terminator, ok := r.exports.Infrastructure.EventHub.(interface{ Terminate(context.Context) }); ok {
		terminator.Terminate(ctx)
	}
	return nil
}

func (r *Runtime) Exports() Exports {
	if r == nil {
		return Exports{}
	}
	return r.exports
}

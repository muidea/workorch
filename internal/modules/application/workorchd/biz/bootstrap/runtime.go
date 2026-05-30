package bootstrap

import (
	"context"

	approvalmodule "github.com/muidea/workorch/internal/modules/kernel/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	eventstreammodule "github.com/muidea/workorch/internal/modules/kernel/eventstream"
	mcpmodule "github.com/muidea/workorch/internal/modules/kernel/mcp"
	memorymodule "github.com/muidea/workorch/internal/modules/kernel/memory"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	skillmodule "github.com/muidea/workorch/internal/modules/kernel/skill"
	soulmodule "github.com/muidea/workorch/internal/modules/kernel/soul"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type lifecycle interface {
	ID() string
	Run() error
	Teardown(context.Context) error
}

type Runtime struct {
	exports         Exports
	modules         []lifecycle
	moduleLifecycle *modulePluginLifecycle
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
	ArtifactContinuations repository.ArtifactContinuationRepository
	Tickets               repository.DecisionTicketRepository
	Resolutions           repository.DecisionResolutionRepository
	Approvals             repository.ApprovalRepository
}

type ServiceExports struct {
	WorkspaceService   workspacemodule.Service
	SoulService        soulmodule.Service
	MemoryService      memorymodule.Service
	SkillService       skillmodule.Service
	MCPService         mcpmodule.Service
	EventStreamService eventstreammodule.Service
	DecisionService    decisionmodule.Service
	ApprovalService    approvalmodule.Service
	RunService         runmodule.Service
}

type InfrastructureExports struct {
	EventHub events.Hub
}

type Exports struct {
	Repositories   RepositoryExports
	Services       ServiceExports
	Infrastructure InfrastructureExports
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r.moduleLifecycle != nil && len(r.modules) == len(r.moduleLifecycle.modules) {
		if err := r.moduleLifecycle.Shutdown(ctx); err != nil {
			return err
		}
		r.moduleLifecycle = nil
	} else {
		for idx := len(r.modules) - 1; idx >= 0; idx-- {
			if err := r.modules[idx].Teardown(ctx); err != nil {
				return err
			}
		}
	}
	r.modules = nil
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

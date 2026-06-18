package bootstrap

import (
	"context"
	"errors"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type lifecycle interface {
	ID() string
	Start() error
	Stop(context.Context) error
}

type Runtime struct {
	exports         Exports
	modules         []lifecycle
	moduleLifecycle *runtimeModuleLifecycle
	background      magictask.BackgroundRoutine
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
	var shutdownErr error
	if r.background != nil {
		r.background.Shutdown(ctx)
		r.background = nil
	}
	if r.moduleLifecycle != nil && len(r.modules) == len(r.moduleLifecycle.modules) {
		if err := r.moduleLifecycle.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
		r.moduleLifecycle = nil
	} else {
		for idx := len(r.modules) - 1; idx >= 0; idx-- {
			if err := r.modules[idx].Stop(ctx); err != nil {
				shutdownErr = errors.Join(shutdownErr, err)
			}
		}
	}
	r.modules = nil
	if terminator, ok := r.exports.Infrastructure.EventHub.(interface{ Terminate(context.Context) }); ok {
		terminator.Terminate(ctx)
	}
	return shutdownErr
}

func (r *Runtime) Exports() Exports {
	if r == nil {
		return Exports{}
	}
	return r.exports
}

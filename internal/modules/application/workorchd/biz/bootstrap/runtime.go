package bootstrap

import (
	"context"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
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
	Workspaces  repository.WorkspaceRepository
	Runs        repository.RunRepository
	Steps       repository.StepRepository
	Tickets     repository.DecisionTicketRepository
	Resolutions repository.DecisionResolutionRepository
	Approvals   repository.ApprovalRepository
}

type ServiceExports struct {
	WorkspaceService workspacemodule.Service
	DecisionService  decisionmodule.Service
	ApprovalService  approvalmodule.Service
	RunService       runmodule.Service
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
	if terminator, ok := r.exports.Infrastructure.EventHub.(interface{ Terminate() }); ok {
		terminator.Terminate()
	}
	return nil
}

func (r *Runtime) Exports() Exports {
	if r == nil {
		return Exports{}
	}
	return r.exports
}

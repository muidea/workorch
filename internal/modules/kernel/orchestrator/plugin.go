package orchestrator

import (
	"context"

	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 30 }

// TeardownWeight stops Runtime drive workers after the application ingress is
// closed but before Run and capability owners dismantle their repositories and
// command subscriptions. Startup remains Weight 30 so Run recovery still sees
// a fully subscribed Orchestrator.
func (m *Module) TeardownWeight() int { return 90 }

func (m *Module) Setup(_ context.Context, rawEventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if rawEventHub == nil || backgroundRoutine == nil {
		return cd.NewError(cd.IllegalParam, "event hub and background routine are required")
	}
	repos, err := workorchdeps.ResolveOrchestratorRepositories(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.Configure(orchestratorbiz.Dependencies{
		Evidence:          repos,
		CommandHub:        rawEventHub,
		BackgroundRoutine: backgroundRoutine,
	}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (m *Module) Run(ctx context.Context) *cd.Error {
	if m == nil || m.bizPtr == nil || ctx == nil {
		return cd.NewError(cd.IllegalParam, "orchestrator module is not configured")
	}
	if err := m.bizPtr.StartContext(ctx); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m != nil {
		_ = m.Stop(ctx)
	}
}

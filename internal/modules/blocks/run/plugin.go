package run

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 40 }

func (m *Module) Setup(_ context.Context, eventHub event.Hub, background task.BackgroundRoutine) *cd.Error {
	repos, err := workorchdeps.ResolveRunRepositories(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.Configure(Dependencies{
		Runs:              repos.Runs,
		Steps:             repos.Steps,
		Checkpoints:       repos.Checkpoints,
		EventHub:          eventHub,
		BackgroundRoutine: background,
	}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	// GMM-R1 / SM-4: Orchestrator runs before this module by framework weight.
	// The recovery inventory is therefore a Setup-phase read contract rather than
	// a normal Run-phase command subscription.
	if setupErr := m.prepareRuntimeRecoveryInventory(); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "run module is not configured")
	}
	if err := m.Start(); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m != nil {
		_ = m.Stop(ctx)
	}
}

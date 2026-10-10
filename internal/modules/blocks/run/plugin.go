package run

import (
	"context"

	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"

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
	if setupErr := m.Configure(runbiz.Dependencies{
		OwnerRepositories: repos.Owner,
		EventHub:          eventHub,
		BackgroundRoutine: background,
	}); setupErr != nil {
		return cd.WrapError(cd.Unexpected, setupErr, "run owner setup failed")
	}
	// GMM-R1 / SM-4: Orchestrator runs before this module by framework weight.
	// The recovery inventory is therefore a Setup-phase read contract rather than
	// a normal Run-phase command subscription.
	if setupErr := m.prepareRuntimeRecoveryInventory(); setupErr != nil {
		return cd.WrapError(cd.Unexpected, setupErr, "run owner setup failed")
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "run module is not configured")
	}
	if err := m.Start(); err != nil {
		return cd.WrapError(cd.Unexpected, err, "run owner start failed")
	}
	return nil
}

// BeginShutdown/Quiesce 只暴露 Run owner 的分阶段生命周期。
// 全进程 Quiescer 切换与共享 Hub/task 屏障仍由调用方负责。
func (m *Module) BeginShutdown(_ context.Context) {
	if m != nil && m.bizPtr != nil {
		m.bizPtr.BeginShutdown()
	}
}

func (m *Module) Quiesce(ctx context.Context) *cd.Error {
	if m == nil || m.bizPtr == nil {
		return nil
	}
	if err := m.bizPtr.Quiesce(ctx); err != nil {
		return cd.WrapError(cd.Unexpected, err, "run owner quiescence failed")
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m != nil {
		if err := m.Stop(ctx); err != nil {
			panic(cd.WrapError(cd.Unexpected, err, "run owner teardown failed"))
		}
	}
}

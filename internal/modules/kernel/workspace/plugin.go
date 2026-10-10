package workspace

import (
	"context"

	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 20 }

func (m *Module) Setup(_ context.Context, eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	repos, err := workorchdeps.ResolveWorkspaceRepositories(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.configure(workspacebiz.Dependencies{
		OwnerRepositories: repos.Owner,
		EventHub:          eventHub,
		BackgroundRoutine: backgroundRoutine,
	}); setupErr != nil {
		return cd.WrapError(cd.Unexpected, setupErr, "workspace setup failed")
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "workspace block is not configured")
	}
	if err := m.Start(); err != nil {
		return cd.WrapError(cd.Unexpected, err, "workspace start failed")
	}
	return nil
}

func (m *Module) BeginShutdown(ctx context.Context) {
	if m != nil && m.bizPtr != nil {
		m.bizPtr.BeginShutdown(ctx)
	}
}

func (m *Module) Quiesce(ctx context.Context) *cd.Error {
	if m != nil && m.bizPtr != nil {
		if err := m.bizPtr.Quiesce(ctx); err != nil {
			return cd.WrapError(cd.Unexpected, err, "workspace quiesce failed")
		}
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if err := m.Stop(ctx); err != nil {
		panic(cd.WrapError(cd.Unexpected, err, "workspace teardown failed"))
	}
}

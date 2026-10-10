package system

import (
	"context"

	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 29 }

func (m *Module) Setup(_ context.Context, eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if setupErr := m.configure(systembiz.Dependencies{EventHub: eventHub, BackgroundRoutine: backgroundRoutine}); setupErr != nil {
		return cd.WrapError(cd.Unexpected, setupErr, "system setup failed")
	}
	return nil
}

func (m *Module) Run(ctx context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "system module is not configured")
	}
	if err := m.Start(ctx); err != nil {
		return cd.WrapError(cd.Unexpected, err, "system start failed")
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
			return cd.WrapError(cd.Unexpected, err, "system quiesce failed")
		}
	}
	return nil
}
func (m *Module) Teardown(ctx context.Context) {
	if err := m.Stop(ctx); err != nil {
		panic(cd.WrapError(cd.Unexpected, err, "system teardown failed"))
	}
}

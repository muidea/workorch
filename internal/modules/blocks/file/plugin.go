package file

import (
	"context"

	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 14 }

func (m *Module) Setup(_ context.Context, eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if setupErr := m.configure(filebiz.Dependencies{EventHub: eventHub, BackgroundRoutine: backgroundRoutine}); setupErr != nil {
		return cd.WrapError(cd.Unexpected, setupErr, "file setup failed")
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "file module is not configured")
	}
	if err := m.Start(); err != nil {
		return cd.WrapError(cd.Unexpected, err, "file start failed")
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m != nil {
		if err := m.Stop(ctx); err != nil {
			panic(cd.WrapError(cd.Unexpected, err, "file teardown failed"))
		}
	}
}
func (m *Module) BeginShutdown(ctx context.Context) {
	if m != nil && m.bizPtr != nil {
		m.bizPtr.BeginShutdown(ctx)
	}
}
func (m *Module) Quiesce(ctx context.Context) *cd.Error {
	if m == nil || m.bizPtr == nil {
		return nil
	}
	if err := m.bizPtr.Quiesce(ctx); err != nil {
		return cd.WrapError(cd.Unexpected, err, "file quiescence failed")
	}
	return nil
}

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
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (m *Module) Run(ctx context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "system module is not configured")
	}
	if err := m.Start(ctx); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m != nil {
		_ = m.Stop(ctx)
	}
}

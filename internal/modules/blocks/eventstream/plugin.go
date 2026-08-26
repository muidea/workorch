package eventstream

import (
	"context"

	eventstreambiz "github.com/muidea/workorch/internal/modules/blocks/eventstream/biz"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 22 }

func (m *Module) Setup(_ context.Context, eventHub event.Hub, background task.BackgroundRoutine) *cd.Error {
	repos, err := workorchdeps.ResolveEventStreamRepositories(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.Configure(eventstreambiz.Dependencies{
		EventRecords:       repos.EventRecords,
		WorkspaceDeletions: repos.WorkspaceDeletions,
		WorkspacePurger:    repos.WorkspacePurger,
		EventHub:           eventHub,
		BackgroundRoutine:  background,
	}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "eventstream module is not configured")
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

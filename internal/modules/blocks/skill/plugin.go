package skill

import (
	"context"

	skillbiz "github.com/muidea/workorch/internal/modules/blocks/skill/biz"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 27 }

func (m *Module) Setup(_ context.Context, eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	repos, err := workorchdeps.ResolveSkillRepositories(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.configure(skillbiz.Dependencies{
		Skills:            repos.Skills,
		EventHub:          eventHub,
		BackgroundRoutine: backgroundRoutine,
	}); setupErr != nil {
		return cd.WrapError(cd.Unexpected, setupErr, "skill setup failed")
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "skill block is not configured")
	}
	if err := m.Start(); err != nil {
		return cd.WrapError(cd.Unexpected, err, "skill start failed")
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m != nil {
		if err := m.Stop(ctx); err != nil {
			panic(cd.WrapError(cd.Unexpected, err, "skill teardown failed"))
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
		return cd.WrapError(cd.Unexpected, err, "skill quiescence failed")
	}
	return nil
}

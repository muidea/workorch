package approval

import (
	"context"

	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"

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
	repos, err := workorchdeps.ResolveApprovalRepositories(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.configure(approvalbiz.Dependencies{
		OwnerRepositories: repos.Owner,
		EventHub:          eventHub,
		BackgroundRoutine: backgroundRoutine,
	}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "approval block is not configured")
	}
	if err := m.Start(); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m != nil {
		if err := m.Stop(ctx); err != nil {
			panic(cd.WrapError(cd.Unexpected, err, "approval shutdown failed"))
		}
	}
}

package governance

import (
	"context"

	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 45 }

func (m *Module) Setup(_ context.Context, eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	repos, err := workorchdeps.ResolveGovernanceRepositories(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.Configure(governancebiz.EventDependencies{
		AutoGovernanceTraces: repos.AutoGovernanceTraces,
		EventHub:             eventHub,
		LoadConfig:           workorchconfig.Current,
		BackgroundRoutine:    backgroundRoutine,
	}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "governance module is not configured")
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

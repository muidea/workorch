package mcp

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

func (m *Module) Weight() int { return 28 }

func (m *Module) Setup(_ context.Context, _ event.Hub, _ task.BackgroundRoutine) *cd.Error {
	repos, err := workorchdeps.ResolveMCPRepositories(ModuleID)
	if err != nil {
		return err
	}
	eventHub, err := workorchdeps.ResolveEventHub(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.Configure(Dependencies{
		Servers:      repos.Servers,
		Capabilities: repos.Capabilities,
		EventHub:     eventHub,
	}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "mcp module is not configured")
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

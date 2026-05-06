package web

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
)

func init() {
	pluginmodule.Register(newPlugin())
}

type plugin struct {
	module *Module
}

func newPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) ID() string { return ModuleID }

func (p *plugin) Weight() int { return 18 }

func (p *plugin) Setup(_ event.Hub, _ task.BackgroundRoutine) *cd.Error {
	runtime, err := workorchRuntime()
	if err != nil {
		return err
	}
	repos := runtime.Repositories()
	p.module = New()
	if setupErr := p.module.Setup(Dependencies{Approvals: repos.Approvals}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	runtime.SetWebRuntimeAdapter(p.module.RuntimeAdapter())
	return nil
}

func (p *plugin) Run() *cd.Error {
	if p == nil || p.module == nil {
		return cd.NewError(cd.IllegalParam, "web module is not configured")
	}
	if err := p.module.Run(); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (p *plugin) Teardown() {
	if p == nil || p.module == nil {
		return
	}
	_ = p.module.Teardown(context.Background())
	p.module = nil
}

func workorchRuntime() (runtimecommon.Helper, *cd.Error) {
	var helper runtimecommon.Helper
	return initiator.GetEntity(runtimecommon.InitiatorID, helper)
}

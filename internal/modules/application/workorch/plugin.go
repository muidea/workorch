package workorch

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

func (p *plugin) Weight() int { return 110 }

func (p *plugin) Setup(_ context.Context, _ event.Hub, _ task.BackgroundRoutine) *cd.Error {
	_, err := workorchRuntime()
	if err != nil {
		return err
	}
	p.module = New()
	return nil
}

func (p *plugin) Run(_ context.Context) *cd.Error {
	if p == nil || p.module == nil {
		return cd.NewError(cd.IllegalParam, "workorch cli module is not configured")
	}
	if err := p.module.Run(); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (p *plugin) Teardown(ctx context.Context) {
	if p != nil && p.module != nil {
		_ = p.module.Teardown(ctx)
		p.module = nil
	}
}

func workorchRuntime() (runtimecommon.Helper, *cd.Error) {
	var helper runtimecommon.Helper
	return initiator.GetEntity(runtimecommon.InitiatorID, helper)
}

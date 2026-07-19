package workorchd

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	workorchdbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
)

const ModuleID = modulecommon.ModuleID

func init() {
	pluginmodule.Register(New())
}

type Module struct {
	bizPtr *workorchdbiz.Biz
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string {
	return ModuleID
}

func (m *Module) Weight() int {
	return 100
}

func (m *Module) Setup(ctx context.Context, commandHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "workorch module is nil")
	}
	owner := workorchdbiz.New()
	if err := owner.Setup(ctx, commandHub, backgroundRoutine); err != nil {
		return err
	}
	m.bizPtr = owner
	return nil
}

func (m *Module) Run(ctx context.Context) *cd.Error {
	if m == nil || m.bizPtr == nil {
		return cd.NewError(cd.IllegalParam, "workorch module is not configured")
	}
	return m.bizPtr.Start(ctx)
}

func (m *Module) Teardown(ctx context.Context) {
	if m == nil {
		return
	}
	if m.bizPtr != nil {
		m.bizPtr.Stop(ctx)
		m.bizPtr = nil
	}
}

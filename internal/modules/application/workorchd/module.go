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
	// Setup 部分失败也必须保留 owner，交给检查式停机清理已取得的资源。
	m.bizPtr = owner
	if err := owner.Setup(ctx, commandHub, backgroundRoutine); err != nil {
		return err
	}
	return nil
}

func (m *Module) Run(ctx context.Context) *cd.Error {
	if m == nil || m.bizPtr == nil {
		return cd.NewError(cd.IllegalParam, "workorch module is not configured")
	}
	return m.bizPtr.Start(ctx)
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
		return cd.WrapError(cd.Unexpected, err, "workorch quiescence failed")
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m == nil {
		return
	}
	if m.bizPtr != nil {
		if err := m.bizPtr.Stop(ctx); err != nil {
			panic(cd.WrapError(cd.Unexpected, err, "workorch teardown failed"))
		}
		m.bizPtr = nil
	}
}

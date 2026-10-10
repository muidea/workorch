package workorch

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	frameworkservice "github.com/muidea/magicCommon/framework/service"
	"github.com/muidea/magicCommon/task"
)

// 全参与者提供阶段协议后，Application 先排空 owner，再排空共享任务和 Hub，
// 最后委托 checked plugin teardown 与 Initiator 释放。本壳不拥有插件状态。
type frameworkModuleService struct{ delegate frameworkservice.Service }

var _ frameworkservice.Quiescer = (*frameworkModuleService)(nil)
var _ frameworkservice.CheckedShutdown = (*frameworkModuleService)(nil)

func (s *frameworkModuleService) Startup(ctx context.Context, name string, hub event.Hub, background task.BackgroundRoutine) *cd.Error {
	return s.delegate.Startup(ctx, name, hub, background)
}

func (s *frameworkModuleService) Run(ctx context.Context) *cd.Error {
	return s.delegate.Run(ctx)
}

func (s *frameworkModuleService) Quiesce(ctx context.Context) *cd.Error {
	return s.delegate.(frameworkservice.Quiescer).Quiesce(ctx)
}

func (s *frameworkModuleService) Shutdown(ctx context.Context) {
	if err := s.ShutdownChecked(ctx); err != nil {
		panic(err)
	}
}

func (s *frameworkModuleService) ShutdownChecked(ctx context.Context) *cd.Error {
	return s.delegate.(frameworkservice.CheckedShutdown).ShutdownChecked(ctx)
}

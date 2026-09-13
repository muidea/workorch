package workorch

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	frameworkservice "github.com/muidea/magicCommon/framework/service"
	"github.com/muidea/magicCommon/task"
)

// WorkOrch owners cancel and drain their workers in ordered Stop/Teardown.
// They do not yet implement the framework's separate BeginShutdown/Quiesce
// phase. Do not advertise Quiescer by embedding DefaultService: Application
// would wait for those workers before invoking the Stop that cancels them.
// Delegate the checked, ordered plugin shutdown first; Application then drains
// and releases its shared infrastructure. This shell owns no plugin state.
type frameworkModuleService struct{ delegate frameworkservice.Service }

var _ frameworkservice.CheckedShutdown = (*frameworkModuleService)(nil)

func (s *frameworkModuleService) Startup(ctx context.Context, name string, hub event.Hub, background task.BackgroundRoutine) *cd.Error {
	return s.delegate.Startup(ctx, name, hub, background)
}

func (s *frameworkModuleService) Run(ctx context.Context) *cd.Error {
	return s.delegate.Run(ctx)
}

func (s *frameworkModuleService) Shutdown(ctx context.Context) {
	s.delegate.Shutdown(ctx)
}

func (s *frameworkModuleService) ShutdownChecked(ctx context.Context) *cd.Error {
	return s.delegate.(frameworkservice.CheckedShutdown).ShutdownChecked(ctx)
}

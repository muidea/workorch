package workorchd

import (
	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
)

const ModuleID = modulecommon.ModuleID

var defaultModule = New()

func init() {
	pluginmodule.Register(defaultModule)
}

type Module struct {
	service *server.Service
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

func (m *Module) Setup(eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	config, err := server.ConfigFromEnv()
	if err != nil {
		return cd.NewError(cd.IllegalParam, err.Error())
	}
	m.service = server.NewService(config)
	return m.service.Startup("workorchd", eventHub, backgroundRoutine)
}

func (m *Module) Run() *cd.Error {
	if m == nil || m.service == nil {
		return cd.NewError(cd.IllegalParam, "workorchd module is not configured")
	}
	return m.service.Run()
}

func (m *Module) Teardown() {
	if m == nil || m.service == nil {
		return
	}
	m.service.Shutdown()
	m.service = nil
}

func (m *Module) Done() <-chan error {
	if m == nil || m.service == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return m.service.Done()
}

func Done() <-chan error {
	return defaultModule.Done()
}

package workorchd

import (
	"context"
	"fmt"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	"github.com/muidea/workorch/internal/pkg/events"
)

const ModuleID = modulecommon.ModuleID

func init() {
	pluginmodule.Register(New())
}

type Module struct {
	service       *server.Service
	eventHub      events.Hub
	rawEventHub   magicevent.Hub
	session       *appservice.RuntimeSession
	cliDispatcher *appservice.CLICommandDispatcher
	doneObserver  magicevent.SimpleObserver
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

func (m *Module) Setup(ctx context.Context, eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	config, err := server.ConfigFromEnv()
	if err != nil {
		return cd.NewError(cd.IllegalParam, err.Error())
	}
	m.eventHub = events.NewMagicHubWithHubOwner(eventHub, ModuleID)
	m.rawEventHub = eventHub
	m.service = server.NewService(config)
	session, sessionErr := appservice.NewRuntimeSessionFromInitiator()
	if sessionErr != nil {
		return cd.NewError(cd.IllegalParam, sessionErr.Error())
	}
	m.session = session
	if err := m.service.Startup(ctx, modulecommon.ServiceName, eventHub, backgroundRoutine); err != nil {
		_ = m.session.Close(ctx)
		m.session = nil
		m.service = nil
		m.eventHub = nil
		return err
	}
	return nil
}

func (m *Module) Run(ctx context.Context) *cd.Error {
	if m == nil || m.service == nil {
		return cd.NewError(cd.IllegalParam, "workorch module is not configured")
	}
	if err := m.service.Run(ctx); err != nil {
		return cd.NewError(err.Code, fmt.Sprintf("workorch service run failed: %s", err.Message))
	}
	if m.eventHub != nil {
		m.cliDispatcher = appservice.NewCLICommandDispatcher(m.session, m.eventHub, m.rawEventHub, m.service)
		if err := m.cliDispatcher.Run(); err != nil {
			m.cliDispatcher = nil
			m.service.Shutdown(ctx)
			return cd.NewError(cd.IllegalParam, fmt.Sprintf("workorch cli dispatcher run failed: %v", err))
		}
		m.doneObserver = magicevent.NewSimpleObserver(workorchdevents.CommandDestination, m.rawEventHub)
		m.doneObserver.Subscribe(workorchdevents.TopicWorkorchDCommandDone, m.handleDoneCommand)
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m == nil {
		return
	}
	if m.doneObserver != nil {
		m.doneObserver.Unsubscribe(workorchdevents.TopicWorkorchDCommandDone)
		m.doneObserver = nil
	}
	if m.cliDispatcher != nil {
		m.cliDispatcher.Close()
		m.cliDispatcher = nil
	}
	if m.service != nil {
		m.service.Shutdown(ctx)
	}
	if m.session != nil {
		_ = m.session.Close(ctx)
		m.session = nil
	}
	m.service = nil
	m.eventHub = nil
	m.rawEventHub = nil
}

func (m *Module) Done() <-chan error {
	if m == nil || m.service == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return m.service.Done()
}

func (m *Module) handleDoneCommand(ev magicevent.Event, result magicevent.Result) {
	if _, ok := ev.Data().(workorchdevents.DoneCommand); !ok {
		result.Set(nil, cd.NewError(cd.IllegalParam, "invalid workorchd done command"))
		return
	}
	result.Set(workorchdevents.DoneCommandResult{Done: m.Done()}, nil)
}

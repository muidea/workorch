package workorchd

import (
	"context"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
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
	session       *appservice.RuntimeSession
	cliDispatcher *appservice.CLICommandDispatcher
	doneUnsub     func()
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

func (m *Module) Setup(ctx context.Context, eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	config, err := server.ConfigFromEnv()
	if err != nil {
		return cd.NewError(cd.IllegalParam, err.Error())
	}
	m.eventHub = events.NewMagicHubWithHubOwner(eventHub, ModuleID)
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
		return err
	}
	if m.eventHub != nil {
		m.cliDispatcher = appservice.NewCLICommandDispatcher(m.session, m.eventHub)
		if err := m.cliDispatcher.Run(); err != nil {
			m.cliDispatcher = nil
			m.service.Shutdown(ctx)
			return cd.NewError(cd.IllegalParam, err.Error())
		}
		m.doneUnsub = m.eventHub.Subscribe(workorchdevents.TopicWorkorchDCommandDone, m.handleDoneCommand)
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m == nil {
		return
	}
	if m.doneUnsub != nil {
		m.doneUnsub()
		m.doneUnsub = nil
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
}

func (m *Module) Done() <-chan error {
	if m == nil || m.service == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return m.service.Done()
}

func (m *Module) handleDoneCommand(_ context.Context, envelope events.Envelope) error {
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload: map[string]any{
			events.PayloadKeyDoneChannel: m.Done(),
		},
	})
}

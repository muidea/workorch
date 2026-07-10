package eventstream

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	eventstreambiz "github.com/muidea/workorch/internal/modules/kernel/eventstream/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/common"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	eventstreamservice "github.com/muidea/workorch/internal/modules/kernel/eventstream/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = eventstreamservice.Service

type Dependencies struct {
	EventRecords repository.EventRecordRepository
	EventHub     magicevent.Hub
}

type Module struct {
	bizPtr     *eventstreambiz.Service
	servicePtr *eventstreamservice.Module
	eventHub   magicevent.Hub
	observer   magicevent.SimpleObserver
	topics     []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.EventRecords == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = eventstreambiz.NewService(deps.EventRecords, events.NewMagicHubWithHubOwner(deps.EventHub, ModuleID))
	m.servicePtr = eventstreamservice.New(eventstreamservice.Dependencies{
		ListEvents:           m.bizPtr.ListEvents,
		PurgeWorkspaceEvents: m.bizPtr.PurgeWorkspaceEvents,
	})
	m.eventHub = deps.EventHub
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	if err := m.bizPtr.Start(); err != nil {
		return err
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{
		eventstreamevents.TopicEventStreamCommandList,
		eventstreamevents.TopicEventStreamCommandPurgeWorkspace,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.bizPtr != nil {
		m.bizPtr.Stop()
	}
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	if m == nil || m.servicePtr == nil {
		return nil
	}
	return m.servicePtr
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev.Context(), ev.ID(), ev.Data())
	setEventStreamMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (eventstreamevents.EventStreamCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case eventstreamevents.TopicEventStreamCommandList:
		cmd, ok := data.(eventstreamevents.ListEventsCommand)
		if !ok {
			return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
		}
		query := cmd.Query
		if query.WorkspaceID != "" {
			ctx = common.ContextWithRuntimeWorkspaceID(ctx, query.WorkspaceID)
		}
		items, err := m.servicePtr.ListEvents(ctx, query)
		if err != nil {
			return eventstreamevents.EventStreamCommandResult{}, err
		}
		return eventstreamevents.EventStreamCommandResult{Events: items}, nil
	case eventstreamevents.TopicEventStreamCommandPurgeWorkspace:
		cmd, ok := data.(eventstreamevents.PurgeWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
		}
		deleted, err := m.servicePtr.PurgeWorkspaceEvents(ctx, cmd.WorkspaceID)
		if err != nil {
			return eventstreamevents.EventStreamCommandResult{}, err
		}
		return eventstreamevents.EventStreamCommandResult{DeletedCount: deleted}, nil
	default:
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
	}
}

func setEventStreamMagicResult(result magicevent.Result, data eventstreamevents.EventStreamCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

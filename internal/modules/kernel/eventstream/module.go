package eventstream

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	eventstreambiz "github.com/muidea/workorch/internal/modules/kernel/eventstream/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/common"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	EventRecords repository.EventRecordRepository
	EventHub     magicevent.Hub
}

type Module struct {
	bizPtr   *eventstreambiz.Service
	eventHub magicevent.Hub
	observer magicevent.SimpleObserver
	topics   []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.EventRecords == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = eventstreambiz.NewService(deps.EventRecords, deps.EventHub)
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
		eventstreamevents.TopicEventStreamCommandHighWatermarks,
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
	m.bizPtr = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setEventStreamMagicResult(result, response, err)
}

func (m *Module) executeCommand(ev magicevent.Event) (eventstreamevents.EventStreamCommandResult, error) {
	ctx := ev.Context()
	topic := ev.ID()
	data := ev.Data()
	if m == nil || m.bizPtr == nil {
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case eventstreamevents.TopicEventStreamCommandList:
		cmd, ok := data.(eventstreamevents.ListEventsCommand)
		if !ok {
			return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
		}
		query := eventRecordQueryFromEvent(cmd.Query)
		if query.WorkspaceID != "" {
			ctx = common.ContextWithRuntimeWorkspaceID(ctx, query.WorkspaceID)
		}
		items, err := m.bizPtr.ListEvents(ctx, query)
		if err != nil {
			return eventstreamevents.EventStreamCommandResult{}, err
		}
		events := make([]eventstreamevents.EventRecordData, 0, len(items))
		for _, item := range items {
			converted, convertErr := eventstreamevents.EventRecordDataFromModel(item)
			if convertErr != nil {
				return eventstreamevents.EventStreamCommandResult{}, convertErr
			}
			events = append(events, converted)
		}
		return eventstreamevents.EventStreamCommandResult{Events: events}, nil
	case eventstreamevents.TopicEventStreamCommandHighWatermarks:
		cmd, ok := data.(eventstreamevents.HighWatermarksCommand)
		if !ok {
			return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.bizPtr.HighWatermarks(ctx, cmd.WorkspaceIDs)
		if err != nil {
			return eventstreamevents.EventStreamCommandResult{}, err
		}
		return eventstreamevents.EventStreamCommandResult{HighWatermarks: items}, nil
	case eventstreamevents.TopicEventStreamCommandPurgeWorkspace:
		cmd, ok := data.(eventstreamevents.PurgeWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
		}
		deleted, err := m.bizPtr.PurgeWorkspaceEvents(ctx, cmd.WorkspaceID)
		if err != nil {
			return eventstreamevents.EventStreamCommandResult{}, err
		}
		return eventstreamevents.EventStreamCommandResult{DeletedCount: deleted}, nil
	default:
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
	}
}

func eventRecordQueryFromEvent(query eventstreamevents.EventRecordQuery) repository.EventRecordQuery {
	return repository.EventRecordQuery{
		WorkspaceID: query.WorkspaceID, RunID: query.RunID, Topic: query.Topic,
		ExcludeTopics: append([]string(nil), query.ExcludeTopics...), SubjectType: query.SubjectType,
		SubjectID: query.SubjectID, CursorAfterID: query.CursorAfterID, OccurredAfter: query.OccurredAfter,
		OccurredBefore: query.OccurredBefore, Limit: query.Limit, IncludePayload: query.IncludePayload,
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

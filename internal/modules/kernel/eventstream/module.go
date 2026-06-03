package eventstream

import (
	"context"

	eventstreambiz "github.com/muidea/workorch/internal/modules/kernel/eventstream/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/common"
	eventstreamservice "github.com/muidea/workorch/internal/modules/kernel/eventstream/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = eventstreamservice.Service

type Dependencies struct {
	EventRecords repository.EventRecordRepository
	EventHub     events.Hub
}

type Module struct {
	bizPtr     *eventstreambiz.Service
	servicePtr *eventstreamservice.Module
	eventHub   events.Hub
	unsub      []func()
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.EventRecords == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = eventstreambiz.NewService(deps.EventRecords, deps.EventHub)
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
	if len(m.unsub) > 0 {
		return nil
	}
	if err := m.bizPtr.Start(); err != nil {
		return err
	}
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(events.TopicEventStreamCommandList, m.handleListCommand),
		m.eventHub.Subscribe(events.TopicEventStreamCommandPurgeWorkspace, m.handlePurgeWorkspaceCommand),
	)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.bizPtr != nil {
		m.bizPtr.Stop()
	}
	for idx := len(m.unsub) - 1; idx >= 0; idx-- {
		if m.unsub[idx] != nil {
			m.unsub[idx]()
		}
	}
	m.unsub = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleListCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	query := eventRecordQueryFromPayload(envelope.Payload)
	items, err := m.servicePtr.ListEvents(ctx, query)
	if err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload:      map[string]any{events.PayloadKeyItems: items},
	})
}

func (m *Module) handlePurgeWorkspaceCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	if workspaceID == "" {
		workspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	deleted, err := m.servicePtr.PurgeWorkspaceEvents(ctx, workspaceID)
	if err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload:      map[string]any{events.PayloadKeyDeletedCount: deleted},
	})
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func eventRecordQueryFromPayload(payload map[string]any) repository.EventRecordQuery {
	if payload == nil {
		return repository.EventRecordQuery{}
	}
	if query, ok := payload[events.PayloadKeyQuery].(repository.EventRecordQuery); ok {
		return query
	}
	return repository.EventRecordQuery{
		WorkspaceID:    common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID)),
		RunID:          common.RunID(stringPayload(payload, events.PayloadKeyRunID)),
		Topic:          stringPayload(payload, events.PayloadKeyTopic),
		SubjectType:    stringPayload(payload, events.PayloadKeySubjectType),
		SubjectID:      stringPayload(payload, events.PayloadKeySubjectID),
		CursorAfterID:  common.EventRecordID(stringPayload(payload, events.PayloadKeyCursorAfterID)),
		OccurredAfter:  stringPayload(payload, events.PayloadKeyOccurredAfter),
		OccurredBefore: stringPayload(payload, events.PayloadKeyOccurredBefore),
		Limit:          intPayload(payload, events.PayloadKeyLimit),
	}
}

func intPayload(payload map[string]any, key string) int {
	switch value := payload[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

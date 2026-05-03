package backend

import (
	"context"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

// eventHubBackend extends the local backend with EventHub-driven progress
// subscriptions, replacing fixed-interval polling with reactive events.
type eventHubBackend struct {
	*localBackend
	eventHub events.Hub
	evtImpl  *eventHubEventsBackend
}

// NewEventHub creates a Backend using EventHub for progress events.
func NewEventHub(session *appservice.RuntimeSession, eventHub events.Hub) Backend {
	local := NewLocal(session).(*localBackend)
	b := &eventHubBackend{
		localBackend: local,
		eventHub:     eventHub,
	}
	b.evtImpl = &eventHubEventsBackend{parent: b}
	return b
}

// Events overrides the stub to provide EventHub-driven subscriptions.
func (b *eventHubBackend) Events() EventsBackend { return b.evtImpl }

// ── EventHub Events Backend ──

type eventHubEventsBackend struct {
	parent *eventHubBackend
	mu     sync.Mutex
	subs   []func()
}

func (b *eventHubEventsBackend) Subscribe(ctx context.Context, handler EventHandler) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	unsub := b.parent.eventHub.Subscribe("/", func(ctx context.Context, envelope events.Envelope) error {
		evt := convertEnvelope(envelope)
		if evt.Type == "" {
			return nil
		}
		handler(ctx, evt)
		return nil
	})

	b.subs = append(b.subs, unsub)
	return unsub, nil
}

func convertEnvelope(envelope events.Envelope) Event {
	switch envelope.Topic {
	case events.TopicWorkspaceStateChanged:
		return Event{Type: EventWorkspaceStateChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), Payload: envelope.Payload}
	case events.TopicRunCreated, events.TopicRunStateChanged:
		return Event{Type: EventRunStateChanged, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case events.TopicStepDispatched:
		return Event{Type: EventStepDispatched, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case events.TopicStepStateChanged:
		return Event{Type: EventStepCompleted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case events.TopicTicketCreated:
		return Event{Type: EventDecisionCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case events.TopicTicketResolved:
		return Event{Type: EventDecisionResolved, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case events.TopicApprovalCreated:
		return Event{Type: EventApprovalCreated, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case events.TopicApprovalResolved:
		return Event{Type: EventApprovalResolved, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	case events.TopicResultAccepted:
		return Event{Type: EventResultAccepted, WorkspaceID: common.WorkspaceID(envelope.WorkspaceID), RunID: common.RunID(envelope.RunID), Payload: envelope.Payload}
	default:
		return Event{}
	}
}

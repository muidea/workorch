package backend

import (
	"context"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestEventHubBackendDoesNotReportLiveAfterSubscriptionFailure(t *testing.T) {
	hub := magicevent.NewHub(16)
	hub.Terminate(context.Background())
	be := NewEventHub(hub)
	if _, err := be.Events().Subscribe(context.Background(), func(context.Context, port.Event) {}); err == nil {
		t.Fatal("event subscription swallowed closed-Hub error")
	}
	var statuses []port.SubscriptionStatus
	_, err := be.ProjectionEvents().SubscribeWorkspaceProjection(context.Background(),
		func(context.Context, projectioncontract.WorkspaceProjectionChanged) {},
		func(_ context.Context, status port.SubscriptionStatus) { statuses = append(statuses, status) })
	if err == nil {
		t.Fatal("projection subscription swallowed closed-Hub error")
	}
	if len(statuses) != 2 || statuses[0].State != port.SubscriptionConnecting || statuses[1].State != port.SubscriptionDegraded || statuses[1].LastError == "" {
		t.Fatalf("failed subscription states = %+v", statuses)
	}
}

type partialSubscriptionHub struct {
	magicevent.Hub
	registered, unregistered int
	rejection                *cd.Error
}

func (h *partialSubscriptionHub) Subscribe(topic string, observer magicevent.Observer) *cd.Error {
	if topic == workspaceevents.TopicWorkspaceDeleted {
		return h.rejection
	}
	err := h.Hub.Subscribe(topic, observer)
	if err == nil && topic == eventstreamevents.TopicEventStreamRecordAppended {
		h.registered++
	}
	return err
}

func (h *partialSubscriptionHub) Unsubscribe(topic string, observer magicevent.Observer) *cd.Error {
	err := h.Hub.Unsubscribe(topic, observer)
	if err == nil && topic == eventstreamevents.TopicEventStreamRecordAppended {
		h.unregistered++
	}
	return err
}

func TestEventHubBackendRollsBackEarlierSubscriptionOnPartialFailure(t *testing.T) {
	hub := &partialSubscriptionHub{Hub: magicevent.NewHub(16), rejection: cd.NewError(cd.ResourceExhausted, "injected rejection")}
	defer hub.Terminate(context.Background())
	be := NewEventHub(hub)
	unsubscribe, err := be.Events().Subscribe(context.Background(), func(context.Context, port.Event) {})
	if err != hub.rejection || unsubscribe != nil || hub.registered != 1 || hub.unregistered != 1 {
		t.Fatalf("partial subscription not rolled back: error=%v registered=%d unregistered=%d", err, hub.registered, hub.unregistered)
	}
}

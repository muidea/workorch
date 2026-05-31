package events

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
)

func TestMagicHubPublishesThroughMagicCommonHub(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var received Envelope
	unsubscribe := hub.Subscribe(TopicRunStateChanged, func(_ context.Context, envelope Envelope) error {
		received = envelope
		return nil
	})
	defer unsubscribe()

	err := hub.Publish(context.Background(), Envelope{
		Topic: TopicRunStateChanged,
		RunID: "run-1",
		Payload: map[string]any{
			PayloadKeyState: "completed",
		},
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if received.Topic != TopicRunStateChanged {
		t.Fatalf("received topic = %s, want %s", received.Topic, TopicRunStateChanged)
	}
	if received.ID == "" || received.OccurredAt == "" || received.Version == "" {
		t.Fatalf("received envelope was not prepared: %+v", received)
	}
}

func TestMagicHubSupportsWildcardSubscription(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var topics []string
	unsubscribe := hub.Subscribe("*", func(_ context.Context, envelope Envelope) error {
		topics = append(topics, envelope.Topic)
		return nil
	})
	defer unsubscribe()

	if err := hub.Publish(context.Background(), Envelope{Topic: TopicWorkspaceCreated}); err != nil {
		t.Fatalf("publish workspace event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: TopicRunCreated}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}
	if len(topics) != 2 {
		t.Fatalf("topics count = %d, want 2: %+v", len(topics), topics)
	}
}

func TestMagicHubUnsubscribeStopsDelivery(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var calls int
	unsubscribe := hub.Subscribe(TopicApprovalResolved, func(_ context.Context, _ Envelope) error {
		calls++
		return nil
	})
	unsubscribe()

	if err := hub.Publish(context.Background(), Envelope{Topic: TopicApprovalResolved}); err != nil {
		t.Fatalf("publish after unsubscribe: %v", err)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}

func TestMagicHubUnsubscribeDoesNotBlockOnUnderlyingHub(t *testing.T) {
	hub := NewMagicHubWithHub(blockingUnsubscribeHub{})
	unsubscribe := hub.Subscribe(TopicApprovalResolved, func(_ context.Context, _ Envelope) error {
		return nil
	})

	done := make(chan struct{})
	go func() {
		unsubscribe()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(defaultMagicHubUnsubscribeTimeout + 200*time.Millisecond):
		t.Fatal("unsubscribe blocked on underlying magicCommon hub")
	}
}

func TestMagicHubReturnsHandlerError(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	wantErr := errors.New("handler failed")
	unsubscribe := hub.Subscribe(TopicTicketResolved, func(_ context.Context, _ Envelope) error {
		return wantErr
	})
	defer unsubscribe()

	err := hub.Publish(context.Background(), Envelope{Topic: TopicTicketResolved})
	if err == nil {
		t.Fatalf("Publish returned nil, want error")
	}
	if !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("Publish returned unexpected error: %v", err)
	}
}

func TestMagicHubPublishCanReturnHandlerResponse(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	want := Envelope{Topic: TopicCLIResult, Payload: map[string]any{"ok": true}}
	unsubscribe := hub.Subscribe(TopicCLICommand, func(_ context.Context, _ Envelope) error {
		return Response(want)
	})
	defer unsubscribe()

	err := hub.Publish(context.Background(), Envelope{Topic: TopicCLICommand})
	response, ok := ResponseEnvelope(err)
	if !ok {
		t.Fatalf("Publish err = %v, want response envelope", err)
	}
	if response.Topic != TopicCLIResult || response.Payload["ok"] != true {
		t.Fatalf("response = %+v, want CLI result payload", response)
	}
}

type blockingUnsubscribeHub struct{}

func (blockingUnsubscribeHub) Subscribe(string, magicevent.Observer) {}

func (blockingUnsubscribeHub) Unsubscribe(string, magicevent.Observer) {
	select {}
}

func (blockingUnsubscribeHub) Post(magicevent.Event) {}

func (blockingUnsubscribeHub) Send(magicevent.Event) magicevent.Result {
	return nil
}

func (blockingUnsubscribeHub) Terminate(_ context.Context) {}

type recordingLaneHub struct {
	events []magicevent.Event
	lanes  []string
}

func (h *recordingLaneHub) Subscribe(string, magicevent.Observer) {}

func (h *recordingLaneHub) Unsubscribe(string, magicevent.Observer) {}

func (h *recordingLaneHub) Post(event magicevent.Event) {
	h.events = append(h.events, event)
	h.lanes = append(h.lanes, event.LaneKey())
}

func (h *recordingLaneHub) Send(event magicevent.Event) magicevent.Result {
	h.events = append(h.events, event)
	h.lanes = append(h.lanes, event.LaneKey())
	return nil
}

func (h *recordingLaneHub) Terminate(_ context.Context) {}

func TestMagicHubPublishWithoutSubscribersSucceeds(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	if err := hub.Publish(context.Background(), Envelope{Topic: TopicStepDispatched}); err != nil {
		t.Fatalf("Publish without subscribers returned error: %v", err)
	}
	published := hub.Published()
	if len(published) != 1 || published[0].Topic != TopicStepDispatched {
		t.Fatalf("published = %+v, want one step dispatched event", published)
	}
}

func TestMagicHubUsesWorkspaceLaneForWorkspaceScopedEvents(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{Topic: TopicRunStateChanged, WorkspaceID: "ws-a", RunID: "run-001"}); err != nil {
		t.Fatalf("publish workspace a event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: TopicStepStateChanged, WorkspaceID: "ws-b", RunID: "run-001"}); err != nil {
		t.Fatalf("publish workspace b event: %v", err)
	}
	if len(rawHub.lanes) != 2 || rawHub.lanes[0] != "workspace/ws-a" || rawHub.lanes[1] != "workspace/ws-b" {
		t.Fatalf("lanes = %v, want workspace-scoped lanes", rawHub.lanes)
	}
}

func TestMagicHubFallsBackToTopicLaneForGlobalEvents(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{Topic: TopicCLICommand}); err != nil {
		t.Fatalf("publish global event: %v", err)
	}
	if len(rawHub.lanes) != 1 || rawHub.lanes[0] != "topic/"+TopicCLICommand {
		t.Fatalf("lanes = %v, want topic fallback lane", rawHub.lanes)
	}
}

func TestMagicHubSupportsSubscribeWorkorch(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var calls int
	unsubscribers := SubscribeWorkorch(hub, func(_ context.Context, event WorkorchEvent) error {
		calls++
		if event.Topic() != TopicRunStateChanged {
			t.Fatalf("topic = %s, want %s", event.Topic(), TopicRunStateChanged)
		}
		return nil
	})
	defer func() {
		for _, unsubscribe := range unsubscribers {
			if unsubscribe != nil {
				unsubscribe()
			}
		}
	}()

	if err := hub.Publish(context.Background(), Envelope{
		Version: WorkorchEventVersion,
		Topic:   TopicRunStateChanged,
		RunID:   "run-1",
		Payload: RunPayload("", "", "running", "", "", ""),
	}); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestMagicHubDeliversSameTopicToMultipleSubscribers(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var firstCalls int
	var secondCalls int
	unsubscribeFirst := hub.Subscribe(TopicRunStateChanged, func(_ context.Context, _ Envelope) error {
		firstCalls++
		return nil
	})
	defer unsubscribeFirst()
	unsubscribeSecond := hub.Subscribe(TopicRunStateChanged, func(_ context.Context, _ Envelope) error {
		secondCalls++
		return nil
	})
	defer unsubscribeSecond()

	if err := hub.Publish(context.Background(), Envelope{Topic: TopicRunStateChanged}); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("calls = %d/%d, want 1/1", firstCalls, secondCalls)
	}
}

func TestMagicHubPublishesAcrossWrappersSharingSameUnderlyingHub(t *testing.T) {
	rawHub := magicevent.NewHub(defaultMagicHubCapacity)
	defer rawHub.Terminate(context.Background())

	publisher := NewMagicHubWithHubOwner(rawHub, "test/publisher")
	subscriber := NewMagicHubWithHubOwner(rawHub, "test/subscriber")

	var received Envelope
	unsubscribe := subscriber.Subscribe(TopicWorkspaceCreated, func(_ context.Context, envelope Envelope) error {
		received = envelope
		return nil
	})
	defer unsubscribe()

	if err := publisher.Publish(context.Background(), Envelope{Topic: TopicWorkspaceCreated}); err != nil {
		t.Fatalf("publish across wrappers returned error: %v", err)
	}
	if received.Topic != TopicWorkspaceCreated {
		t.Fatalf("received topic = %s, want %s", received.Topic, TopicWorkspaceCreated)
	}
}

func TestMagicHubDeliversSameTopicAcrossMultipleWrappers(t *testing.T) {
	rawHub := magicevent.NewHub(defaultMagicHubCapacity)
	defer rawHub.Terminate(context.Background())

	first := NewMagicHubWithHubOwner(rawHub, "test/first")
	second := NewMagicHubWithHubOwner(rawHub, "test/second")
	publisher := NewMagicHubWithHubOwner(rawHub, "test/publisher")

	var firstCalls int
	var secondCalls int
	unsubscribeFirst := first.Subscribe(TopicWorkspaceCreated, func(_ context.Context, _ Envelope) error {
		firstCalls++
		return nil
	})
	defer unsubscribeFirst()
	unsubscribeSecond := second.Subscribe(TopicWorkspaceCreated, func(_ context.Context, _ Envelope) error {
		secondCalls++
		return nil
	})
	defer unsubscribeSecond()

	if err := publisher.Publish(context.Background(), Envelope{Topic: TopicWorkspaceCreated}); err != nil {
		t.Fatalf("publish across wrappers returned error: %v", err)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("calls = %d/%d, want 1/1", firstCalls, secondCalls)
	}
}

func TestMagicHubDeliversWildcardEventsToMultipleWrappers(t *testing.T) {
	rawHub := magicevent.NewHub(defaultMagicHubCapacity)
	defer rawHub.Terminate(context.Background())

	first := NewMagicHubWithHubOwner(rawHub, "test/first")
	second := NewMagicHubWithHubOwner(rawHub, "test/second")
	publisher := NewMagicHubWithHubOwner(rawHub, "test/publisher")

	var firstCalls int
	var secondCalls int
	unsubscribeFirst := first.Subscribe("*", func(_ context.Context, _ Envelope) error {
		firstCalls++
		return nil
	})
	defer unsubscribeFirst()
	unsubscribeSecond := second.Subscribe("*", func(_ context.Context, _ Envelope) error {
		secondCalls++
		return nil
	})
	defer unsubscribeSecond()

	if err := publisher.Publish(context.Background(), Envelope{Topic: TopicWorkspaceCreated}); err != nil {
		t.Fatalf("publish across wrappers returned error: %v", err)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("wildcard calls = %d/%d, want 1/1", firstCalls, secondCalls)
	}
}

func TestMagicHubObserverIDUsesStableOwnerName(t *testing.T) {
	hub := NewMagicHubWithOwner("workorchd/http")
	defer hub.Terminate(context.Background())

	unsubscribe := hub.Subscribe(TopicWorkspaceCreated, func(_ context.Context, _ Envelope) error {
		return nil
	})
	defer unsubscribe()

	observer := hub.observers[TopicWorkspaceCreated]
	if observer == nil {
		t.Fatal("workspace observer was not registered")
	}
	want := "/_workorch_observer/workorchd/http/workspace/created"
	if observer.ID() != want {
		t.Fatalf("observer ID = %q, want %q", observer.ID(), want)
	}
}

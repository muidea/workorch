package events

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMagicHubPublishesThroughMagicCommonHub(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate()

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
	defer hub.Terminate()

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
	defer hub.Terminate()

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

func TestMagicHubReturnsHandlerError(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate()

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

func TestMagicHubPublishWithoutSubscribersSucceeds(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate()

	if err := hub.Publish(context.Background(), Envelope{Topic: TopicStepDispatched}); err != nil {
		t.Fatalf("Publish without subscribers returned error: %v", err)
	}
	published := hub.Published()
	if len(published) != 1 || published[0].Topic != TopicStepDispatched {
		t.Fatalf("published = %+v, want one step dispatched event", published)
	}
}

func TestMagicHubSupportsSubscribeWorkorch(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate()

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
	defer hub.Terminate()

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

package observed

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
)

func TestCollectorCollectsSubscribedWorkorchEvents(t *testing.T) {
	hub := events.NewMemoryHub()
	collector := NewCollector(hub)
	defer collector.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-1",
		RunID:       "run-1",
		Payload:     map[string]any{events.PayloadKeyState: "running"},
	}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}
	if err := hub.Publish(context.Background(), events.Envelope{
		Topic:       "/unrelated/topic",
		SubjectType: "other",
		SubjectID:   "other-1",
		Payload:     map[string]any{events.PayloadKeyState: "ignored"},
	}); err != nil {
		t.Fatalf("publish unrelated event: %v", err)
	}

	published := collector.PublishedWorkorchEvents()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic() != events.TopicRunStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic(), events.TopicRunStateChanged)
	}
	if published[0].Envelope().Payload[events.PayloadKeyState] != "running" {
		t.Fatalf("payload state = %v, want running", published[0].Envelope().Payload[events.PayloadKeyState])
	}
}

func TestCollectorCloseStopsFurtherCollection(t *testing.T) {
	hub := events.NewMemoryHub()
	collector := NewCollector(hub)
	collector.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   "approval-1",
		Payload:     map[string]any{events.PayloadKeyState: "pending"},
	}); err != nil {
		t.Fatalf("publish approval event: %v", err)
	}

	if published := collector.PublishedWorkorchEvents(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}

func TestCollectorSkipsInvalidWorkorchPayload(t *testing.T) {
	hub := events.NewMemoryHub()
	collector := NewCollector(hub)
	defer collector.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-1",
		RunID:       "run-1",
		Payload: map[string]any{
			events.PayloadKeyState: 123,
		},
	}); err != nil {
		t.Fatalf("publish invalid run event: %v", err)
	}

	if published := collector.PublishedWorkorchEvents(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}

func TestCollectorPublishedWorkorchEventsReturnsDeepClonedPayloads(t *testing.T) {
	hub := events.NewMemoryHub()
	collector := NewCollector(hub)
	defer collector.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicStepStateChanged,
		SubjectType: "step",
		SubjectID:   "run-1/step-1",
		RunID:       "run-1",
		StepID:      "run-1/step-1",
		Payload: events.StepPayload(
			"legacy/minimal",
			"completed",
			map[string]any{
				"workspaceId": "ws-1",
				"items":       []any{"first", map[string]any{"nested": "original"}},
			},
			map[string]any{
				"status": "completed",
			},
		),
	}); err != nil {
		t.Fatalf("publish step event: %v", err)
	}

	first := collector.PublishedWorkorchEvents()
	if len(first) != 1 {
		t.Fatalf("len(first) = %d, want 1", len(first))
	}

	input := first[0].Envelope().Payload[events.PayloadKeyInput].(map[string]any)
	input["workspaceId"] = "ws-mutated"
	items := input["items"].([]any)
	items[0] = "changed"
	items[1].(map[string]any)["nested"] = "mutated"
	output := first[0].Envelope().Payload[events.PayloadKeyOutput].(map[string]any)
	output["status"] = "failed"

	second := collector.PublishedWorkorchEvents()
	if len(second) != 1 {
		t.Fatalf("len(second) = %d, want 1", len(second))
	}

	secondInput := second[0].Envelope().Payload[events.PayloadKeyInput].(map[string]any)
	if secondInput["workspaceId"] != "ws-1" {
		t.Fatalf("workspaceId = %v, want ws-1", secondInput["workspaceId"])
	}
	secondItems := secondInput["items"].([]any)
	if secondItems[0] != "first" {
		t.Fatalf("items[0] = %v, want first", secondItems[0])
	}
	if secondItems[1].(map[string]any)["nested"] != "original" {
		t.Fatalf("nested item = %v, want original", secondItems[1].(map[string]any)["nested"])
	}
	secondOutput := second[0].Envelope().Payload[events.PayloadKeyOutput].(map[string]any)
	if secondOutput["status"] != "completed" {
		t.Fatalf("status = %v, want completed", secondOutput["status"])
	}
}

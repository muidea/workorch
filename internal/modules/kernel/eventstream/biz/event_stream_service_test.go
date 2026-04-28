package eventstream

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestServicePersistsWorkorchEventsFromHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	hub := events.NewMemoryHub()
	service := NewService(repo, hub)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	if err := hub.Publish(ctx, events.Envelope{
		ID:           "event-stream-run-created",
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunCreated,
		SourceModule: "kernel/run",
		SubjectType:  "run",
		SubjectID:    "run-event-stream",
		WorkspaceID:  "ws-event-stream",
		RunID:        "run-event-stream",
		Payload: events.RunPayload(
			common.GoalID("goal-event-stream"),
			common.DoDID("dod-event-stream"),
			common.RunStateCreated,
			"",
			"",
			"",
		),
	}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{RunID: "run-event-stream"})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(records) != 1 || records[0].ID != "event-stream-run-created" || records[0].Payload[events.PayloadKeyState] != string(common.RunStateCreated) {
		t.Fatalf("records = %+v, want persisted run-created event", records)
	}
}

func TestServiceIgnoresNonWorkorchEvents(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	hub := events.NewMemoryHub()
	service := NewService(repo, hub)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	if err := hub.Publish(ctx, events.Envelope{
		ID:           "ignored-event",
		Version:      "custom.v1",
		Topic:        "/custom/event",
		SourceModule: "test",
		SubjectType:  "custom",
		SubjectID:    "custom-1",
	}); err != nil {
		t.Fatalf("publish custom event: %v", err)
	}
	records, err := repo.List(ctx, repository.EventRecordQuery{})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %+v, want no persisted custom event", records)
	}
}

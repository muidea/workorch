package eventstream

import (
	"context"
	"sync"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type countingEventRecordRepository struct {
	base        repository.EventRecordRepository
	mu          sync.Mutex
	appendCount int
}

func newCountingEventRecordRepository() *countingEventRecordRepository {
	return &countingEventRecordRepository{base: memory.NewEventRecordRepository()}
}

func (r *countingEventRecordRepository) Append(ctx context.Context, item *eventstreammodel.EventRecord) error {
	r.mu.Lock()
	r.appendCount++
	r.mu.Unlock()
	return r.base.Append(ctx, item)
}

func (r *countingEventRecordRepository) Get(ctx context.Context, id common.EventRecordID) (*eventstreammodel.EventRecord, error) {
	return r.base.Get(ctx, id)
}

func (r *countingEventRecordRepository) List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return r.base.List(ctx, query)
}

func (r *countingEventRecordRepository) Delete(ctx context.Context, id common.EventRecordID) error {
	return r.base.Delete(ctx, id)
}

func (r *countingEventRecordRepository) AppendCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendCount
}

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
		Topic:        runevents.TopicRunCreated,
		SourceModule: "kernel/run",
		SubjectType:  "run",
		SubjectID:    "run-event-stream",
		WorkspaceID:  "ws-event-stream",
		RunID:        "run-event-stream",
		Payload: events.RunPayload(
			common.GoalID("goal-event-stream"),
			common.DoDID("dod-event-stream"),
			common.RunStatePending,
			"",
			"",
			"",
		),
	}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{RunID: "run-event-stream", IncludePayload: true})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(records) != 1 || records[0].ID != "event-stream-run-created" || records[0].Payload[events.PayloadKeyState] != string(common.RunStatePending) {
		t.Fatalf("records = %+v, want persisted run-created event", records)
	}
}

func TestServiceStartIsIdempotentAndStopReleasesSubscription(t *testing.T) {
	ctx := context.Background()
	repo := newCountingEventRecordRepository()
	hub := events.NewMemoryHub()
	service := NewService(repo, hub)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	if err := service.Start(); err != nil {
		t.Fatalf("second start event stream service: %v", err)
	}
	if err := hub.Publish(ctx, events.Envelope{
		ID:           "event-stream-idempotent-start",
		Version:      events.WorkorchEventVersion,
		Topic:        runevents.TopicRunCreated,
		WorkspaceID:  "ws-event-stream-idempotent",
		RunID:        "run-event-stream-idempotent",
		SourceModule: "kernel/run",
		Payload: events.RunPayload(
			common.GoalID("goal-event-stream-idempotent"),
			common.DoDID("dod-event-stream-idempotent"),
			common.RunStatePending,
			"",
			"",
			"",
		),
	}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}
	if repo.AppendCount() != 1 {
		t.Fatalf("append count after double start = %d, want 1", repo.AppendCount())
	}

	service.Stop()
	if err := hub.Publish(ctx, events.Envelope{
		ID:           "event-stream-after-stop",
		Version:      events.WorkorchEventVersion,
		Topic:        runevents.TopicRunCreated,
		WorkspaceID:  "ws-event-stream-idempotent",
		RunID:        "run-event-stream-idempotent",
		SourceModule: "kernel/run",
		Payload: events.RunPayload(
			common.GoalID("goal-event-stream-idempotent"),
			common.DoDID("dod-event-stream-idempotent"),
			common.RunStatePending,
			"",
			"",
			"",
		),
	}); err != nil {
		t.Fatalf("publish run event after stop: %v", err)
	}
	if repo.AppendCount() != 1 {
		t.Fatalf("append count after stop = %d, want 1", repo.AppendCount())
	}
}

func TestServicePersistsPatchEventsFromHub(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	hub := events.NewMemoryHub()
	service := NewService(repo, hub)
	if err := service.Start(); err != nil {
		t.Fatalf("start event stream service: %v", err)
	}
	defer service.Stop()

	if err := hub.Publish(ctx, events.Envelope{
		ID:           "event-stream-patch-applied",
		Version:      events.WorkorchEventVersion,
		Topic:        orchestratorevents.TopicPatchApplied,
		SourceModule: "kernel/orchestrator",
		SubjectType:  "patch",
		SubjectID:    "patch-event-stream",
		WorkspaceID:  "ws-event-stream",
		RunID:        "run-event-stream",
		StepID:       "run-event-stream/step-1",
		Payload: events.PatchPayload(
			common.PatchID("patch-event-stream"),
			common.StepID("run-event-stream/step-1"),
			common.ToolCallID("run-event-stream/step-1/tool-call"),
			"file_write",
			"result.txt",
			"write result",
			common.PatchStateApplied,
			1,
			0,
			"content-sha",
			"",
			"",
			"",
			"",
		),
	}); err != nil {
		t.Fatalf("publish patch event: %v", err)
	}

	records, err := repo.List(ctx, repository.EventRecordQuery{RunID: "run-event-stream", IncludePayload: true})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(records) != 1 || records[0].Topic != orchestratorevents.TopicPatchApplied || records[0].Payload[events.PayloadKeyTargetRef] != "result.txt" {
		t.Fatalf("records = %+v, want persisted patch event", records)
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

func TestServicePurgeWorkspaceEventsDeletesOnlyWorkspaceRecords(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRecordRepository()
	service := NewService(repo, events.NewMemoryHub())
	for _, envelope := range []events.Envelope{
		{ID: "event-purge-1", Topic: runevents.TopicRunCreated, WorkspaceID: "ws-purge", RunID: "run-purge"},
		{ID: "event-purge-2", Topic: runevents.TopicRunStateChanged, WorkspaceID: "ws-purge", RunID: "run-purge"},
		{ID: "event-keep", Topic: runevents.TopicRunCreated, WorkspaceID: "ws-keep", RunID: "run-keep"},
	} {
		if err := repo.Append(ctx, eventRecordFromEnvelope(envelope)); err != nil {
			t.Fatalf("append event %s: %v", envelope.ID, err)
		}
	}

	deleted, err := service.PurgeWorkspaceEvents(ctx, "ws-purge")
	if err != nil {
		t.Fatalf("purge workspace events: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	records, err := repo.List(ctx, repository.EventRecordQuery{})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(records) != 1 || records[0].ID != "event-keep" {
		t.Fatalf("records = %+v, want only event-keep", records)
	}
}

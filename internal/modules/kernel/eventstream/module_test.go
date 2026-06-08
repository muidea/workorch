package eventstream

import (
	"context"
	"testing"

	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"

	eventstreamcontracts "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/contracts"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type countingSubscribeHub struct {
	subscribeCount   int
	unsubscribeCount int
}

func (h *countingSubscribeHub) Publish(context.Context, events.Envelope) error {
	return nil
}

func (h *countingSubscribeHub) Subscribe(string, events.Handler) func() {
	h.subscribeCount++
	return func() {
		h.unsubscribeCount++
	}
}

func TestModuleRunIsIdempotentAndTeardownUnsubscribes(t *testing.T) {
	ctx := context.Background()
	hub := &countingSubscribeHub{}
	module := New()
	if err := module.Configure(Dependencies{
		EventRecords: memory.NewEventRecordRepository(),
		EventHub:     hub,
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	firstSubscribeCount := hub.subscribeCount
	if firstSubscribeCount == 0 {
		t.Fatalf("subscribe count = 0, want eventstream subscriptions")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if hub.subscribeCount != firstSubscribeCount {
		t.Fatalf("subscribe count after second Run = %d, want %d", hub.subscribeCount, firstSubscribeCount)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if hub.unsubscribeCount != firstSubscribeCount {
		t.Fatalf("unsubscribe count = %d, want %d", hub.unsubscribeCount, firstSubscribeCount)
	}
}

func TestPurgeWorkspaceCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	repo := memory.NewEventRecordRepository()
	workspaceID := common.WorkspaceID("ws-eventstream-contract")
	if err := repo.Append(ctx, &eventstreammodel.EventRecord{ID: "event-contract", WorkspaceID: workspaceID, Topic: "/test"}); err != nil {
		t.Fatalf("seed event record: %v", err)
	}
	module := New()
	if err := module.Configure(Dependencies{EventRecords: repo, EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	err := hub.Publish(ctx, events.Envelope{
		Topic:       eventstreamevents.TopicEventStreamCommandPurgeWorkspace,
		SubjectType: "workspace",
		SubjectID:   string(workspaceID),
		Payload: map[string]any{
			events.PayloadKeyCommand: eventstreamcontracts.PurgeWorkspaceCommand{WorkspaceID: workspaceID},
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("purge err = %v, want response", err)
	}
	items, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("event records after purge = %d, want 0", len(items))
	}
}

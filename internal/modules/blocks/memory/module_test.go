package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/memory" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}

	if err := module.Configure(memoryModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresMemoryItems(t *testing.T) {
	if err := New().Configure(Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure err = %v, want invalid argument", err)
	}
}

func TestMemoryRecallTraceCommandsAcceptContractPayloads(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	traces := memoryrepo.NewMemoryRecallTraceRepository()
	module := New()
	if err := module.Configure(Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: traces,
		EventHub:           hub,
	}); err != nil {
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
	traceID := common.MemoryRecallTraceID("trace-contract")
	const observerID = "/memory/consumers/blocks/memory/module-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	notifications := make(chan memoryevents.MemoryRecallRecordedData, 1)
	observer.Subscribe(memoryevents.TopicMemoryRecallRecorded, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(memoryevents.MemoryRecallRecordedData); ok {
			notifications <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(memoryevents.TopicMemoryRecallRecorded) })

	response := hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallTraceCommandUpsert,
		"test",
		memoryevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		memoryevents.UpsertMemoryRecallTraceCommand{
			TraceID:            traceID,
			WorkspaceID:        "ws-trace-contract",
			RunID:              "run-trace-contract",
			Stage:              "activation",
			CandidateMemoryIDs: []common.MemoryID{"mem-candidate"},
			SelectedMemoryIDs:  []common.MemoryID{"mem-selected"},
			ExcludedMemoryIDs:  []common.MemoryID{"mem-excluded"},
			TypeQuota:          []memoryevents.MemoryTypeQuotaEntry{{Type: "episodic", Quota: 1}},
			SelectionScores: []memoryevents.MemoryRecallSelectionScore{{
				ID:     "mem-excluded",
				Score:  10,
				Reason: "excluded_low_value_process_memory",
			}},
			RetrievedMemoryHits: []memoryevents.MemoryRecallHit{{
				ID:              "mem-hit",
				Summary:         "hit",
				VectorScore:     0.91,
				VectorDimension: 4,
			}},
		},
	))
	if response == nil {
		t.Fatal("upsert response is nil")
	}
	if response.Error() != nil {
		t.Fatalf("upsert error = %v", response.Error())
	}
	select {
	case data := <-notifications:
		if data.TraceID != traceID || data.WorkspaceID != "ws-trace-contract" || data.RunID != "run-trace-contract" || data.Stage != "activation" {
			t.Fatalf("recall notification = %+v, want typed trace scope", data)
		}
	case <-time.After(time.Second):
		t.Fatal("typed memory recall notification not received")
	}
	stored, err := traces.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("stored trace: %v", err)
	}
	if stored.Stage != "activation" || stored.TypeQuota["episodic"] != 1 || len(stored.RetrievedMemoryHits) != 1 || stored.RetrievedMemoryHits[0].ID != "mem-hit" {
		t.Fatalf("stored trace = %+v, want command fields", stored)
	}
	if len(stored.SelectedMemoryIDs) != 1 || stored.SelectedMemoryIDs[0] != "mem-selected" ||
		len(stored.ExcludedMemoryIDs) != 1 || stored.ExcludedMemoryIDs[0] != "mem-excluded" ||
		len(stored.SelectionScores) != 1 || stored.SelectionScores[0].ID != "mem-excluded" ||
		stored.SelectionScores[0].Reason != "excluded_low_value_process_memory" {
		t.Fatalf("stored trace = %+v, want selected/excluded recall audit fields", stored)
	}

	response = hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallTraceCommandDelete,
		"test",
		memoryevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		memoryevents.DeleteMemoryRecallTraceCommand{WorkspaceID: "ws-trace-contract", TraceID: traceID},
	))
	if response == nil {
		t.Fatal("delete response is nil")
	}
	if response.Error() != nil {
		t.Fatalf("delete error = %v", response.Error())
	}
	if _, err := traces.Get(ctx, traceID); err == nil {
		t.Fatalf("trace still exists after delete")
	}
}

func TestMemoryOwnerCommandsPublishLifecycleNotifications(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	const observerID = "/memory/consumers/blocks/memory/owner-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	received := make(chan struct {
		topic string
		data  memoryevents.MemoryChangedData
	}, 3)
	for _, topic := range []string{memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryUpdated, memoryevents.TopicMemoryDeleted} {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(memoryevents.MemoryChangedData); ok {
				received <- struct {
					topic string
					data  memoryevents.MemoryChangedData
				}{topic: topic, data: data}
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	t.Cleanup(func() {
		observer.Unsubscribe(memoryevents.TopicMemoryCreated)
		observer.Unsubscribe(memoryevents.TopicMemoryUpdated)
		observer.Unsubscribe(memoryevents.TopicMemoryDeleted)
	})

	command := memoryevents.CreateMemoryCommand{
		MemoryID: "memory-owner-notify", WorkspaceID: "ws-memory-owner-notify", RunID: "run-memory-owner-notify",
		SourceType: memorycommon.MemorySourceRun, SourceRef: "step-memory-owner-notify",
		MemoryType: memorycommon.MemoryTypeRunSummary, Summary: "created memory", Content: "created content",
		Tags: []string{"owner"}, Importance: 50, Confidence: 60, Visibility: "workspace",
	}
	sendMemoryOwnerCommand(t, ctx, hub, memoryevents.TopicMemoryCommandCreate, command)
	created := receiveMemoryNotification(t, received)
	if created.topic != memoryevents.TopicMemoryCreated || created.data.MemoryID != command.MemoryID || created.data.Summary != command.Summary {
		t.Fatalf("created notification = %+v", created)
	}

	command.Summary = "updated memory"
	sendMemoryOwnerCommand(t, ctx, hub, memoryevents.TopicMemoryCommandUpdate, memoryevents.UpdateMemoryCommand(command))
	updated := receiveMemoryNotification(t, received)
	if updated.topic != memoryevents.TopicMemoryUpdated || updated.data.Summary != command.Summary {
		t.Fatalf("updated notification = %+v", updated)
	}

	sendMemoryOwnerCommand(t, ctx, hub, memoryevents.TopicMemoryCommandDelete, memoryevents.DeleteMemoryCommand{WorkspaceID: command.WorkspaceID, MemoryID: command.MemoryID})
	deleted := receiveMemoryNotification(t, received)
	if deleted.topic != memoryevents.TopicMemoryDeleted || deleted.data.MemoryID != command.MemoryID || deleted.data.WorkspaceID != command.WorkspaceID {
		t.Fatalf("deleted notification = %+v", deleted)
	}
}

func sendMemoryOwnerCommand[Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(topic, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s: %v", topic, result)
	}
}

func receiveMemoryNotification(t *testing.T, received <-chan struct {
	topic string
	data  memoryevents.MemoryChangedData
}) struct {
	topic string
	data  memoryevents.MemoryChangedData
} {
	t.Helper()
	select {
	case item := <-received:
		return item
	case <-time.After(time.Second):
		t.Fatal("typed memory notification not received")
		return struct {
			topic string
			data  memoryevents.MemoryChangedData
		}{}
	}
}

func memoryModuleContractDependencies() Dependencies {
	return Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           magicevent.NewHub(16),
	}
}

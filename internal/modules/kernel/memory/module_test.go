package memory

import (
	"context"
	"errors"
	"testing"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.Service() != nil {
		t.Fatal("new module service is non-nil before Configure")
	}

	if err := module.Configure(memoryModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.Service() == nil {
		t.Fatal("service is nil after Configure")
	}
	if module.bizPtr == nil || module.servicePtr == nil || module.eventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.unsub)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe memory commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.unsub) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.unsub), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.Service() != nil ||
		module.bizPtr != nil ||
		module.servicePtr != nil ||
		module.eventHub != nil ||
		len(module.unsub) != 0 {
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
	module := New()
	if err := module.Configure(Dependencies{MemoryItems: memoryrepo.NewMemoryItemRepository()}); err != nil {
		t.Fatalf("Configure without EventHub: %v", err)
	}
	if module.Service() == nil {
		t.Fatal("service is nil after Configure without EventHub")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start without EventHub should remain no-op: %v", err)
	}
	if len(module.unsub) != 0 {
		t.Fatalf("subscriptions without EventHub = %d, want 0", len(module.unsub))
	}
}

func TestMemoryRecallTraceCommandsAcceptContractPayloads(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       memoryevents.TopicMemoryRecallTraceCommandUpsert,
		SubjectType: "memory_recall_trace",
		SubjectID:   string(traceID),
		WorkspaceID: "ws-trace-contract",
		RunID:       "run-trace-contract",
		Payload: map[string]any{
			events.PayloadKeyCommand: memorycontracts.UpsertMemoryRecallTraceCommand{
				TraceID:            traceID,
				WorkspaceID:        "ws-trace-contract",
				RunID:              "run-trace-contract",
				Stage:              "activation",
				CandidateMemoryIDs: []common.MemoryID{"mem-candidate"},
				SelectedMemoryIDs:  []common.MemoryID{"mem-selected"},
				ExcludedMemoryIDs:  []common.MemoryID{"mem-excluded"},
				TypeQuota:          map[string]int{"episodic": 1},
				SelectionScores: []memorycontracts.MemoryRecallSelectionScoreCommand{{
					ID:     "mem-excluded",
					Score:  10,
					Reason: "excluded_low_value_process_memory",
				}},
				RetrievedMemoryHits: []memorycontracts.MemoryRecallHitCommand{{
					ID:              "mem-hit",
					Summary:         "hit",
					VectorScore:     0.91,
					VectorDimension: 4,
				}},
			},
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("upsert err = %v, want response", err)
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

	err = hub.Publish(ctx, events.Envelope{
		Topic:       memoryevents.TopicMemoryRecallTraceCommandDelete,
		SubjectType: "memory_recall_trace",
		SubjectID:   string(traceID),
		Payload: map[string]any{
			events.PayloadKeyCommand: memorycontracts.DeleteMemoryRecallTraceCommand{TraceID: traceID},
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("delete err = %v, want response", err)
	}
	if _, err := traces.Get(ctx, traceID); err == nil {
		t.Fatalf("trace still exists after delete")
	}
}

func memoryModuleContractDependencies() Dependencies {
	return Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           events.NewMemoryHub(),
	}
}

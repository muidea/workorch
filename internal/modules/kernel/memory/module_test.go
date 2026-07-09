package memory

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"

	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
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
	firstSubscriptionCount := len(module.topics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe memory commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.topics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.topics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.Service() != nil ||
		module.bizPtr != nil ||
		module.servicePtr != nil ||
		module.eventHub != nil ||
		module.observer != nil ||
		len(module.topics) != 0 {
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
	))
	if response == nil {
		t.Fatal("upsert response is nil")
	}
	if response.Error() != nil {
		t.Fatalf("upsert error = %v", response.Error())
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
		memoryevents.DeleteMemoryRecallTraceCommand{TraceID: traceID},
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

func memoryModuleContractDependencies() Dependencies {
	return Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           magicevent.NewHub(16),
	}
}

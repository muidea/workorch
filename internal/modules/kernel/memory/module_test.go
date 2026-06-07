package memory

import (
	"context"
	"testing"

	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

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
		Topic:       events.TopicMemoryRecallTraceCommandUpsert,
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
				TypeQuota:          map[string]int{"episodic": 1},
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

	err = hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicMemoryRecallTraceCommandDelete,
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

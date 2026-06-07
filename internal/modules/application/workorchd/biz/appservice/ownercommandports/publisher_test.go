package ownercommandports

import (
	"context"
	"testing"

	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestPublishMemoryRecallTraceUpsertPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicMemoryRecallTraceCommandUpsert, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})
	trace := &memorymodel.MemoryRecallTrace{
		ID:                 "trace-command-payload",
		WorkspaceID:        "ws-trace-command-payload",
		RunID:              "run-trace-command-payload",
		Stage:              "activation",
		CandidateMemoryIDs: []common.MemoryID{"mem-candidate"},
		TypeQuota:          map[string]int{"episodic": 2},
		RetrievedMemoryHits: []memorymodel.MemoryRecallHit{{
			ID:              "mem-hit",
			Summary:         "hit",
			VectorScore:     0.87,
			VectorDimension: 3,
		}},
	}

	if err := PublishMemoryRecallTraceUpsert(ctx, hub, trace); err != nil {
		t.Fatalf("PublishMemoryRecallTraceUpsert: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(memorycontracts.UpsertMemoryRecallTraceCommand)
	if !ok {
		t.Fatalf("payload command = %T, want UpsertMemoryRecallTraceCommand", published[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.TraceID != trace.ID || cmd.WorkspaceID != trace.WorkspaceID || cmd.RunID != trace.RunID || cmd.TypeQuota["episodic"] != 2 {
		t.Fatalf("command = %+v, want populated trace command", cmd)
	}
	if len(cmd.RetrievedMemoryHits) != 1 || cmd.RetrievedMemoryHits[0].ID != "mem-hit" {
		t.Fatalf("retrieved hits = %+v, want mem-hit", cmd.RetrievedMemoryHits)
	}
	if _, ok := published[0].Payload[events.PayloadKeyMemoryRecallTrace]; ok {
		t.Fatalf("payload contains legacy memoryRecallTrace field")
	}
	if _, ok := published[0].Payload[events.PayloadKeyTraceID]; ok {
		t.Fatalf("payload contains legacy traceId field")
	}
}

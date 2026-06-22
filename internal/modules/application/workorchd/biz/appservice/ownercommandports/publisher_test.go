package ownercommandports

import (
	"context"
	"errors"
	"testing"

	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestPublishRejectsReadTopic(t *testing.T) {
	err := Publish(context.Background(), events.NewMemoryHub(), runevents.TopicRunCommandGet, "run", "run-001", "", "run-001", map[string]any{
		events.PayloadKeyRunID: "run-001",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Publish err = %v, want invalid argument for read topic", err)
	}
}

func TestPublishMemoryRecallTraceUpsertPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert, func(_ context.Context, envelope events.Envelope) error {
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

func TestRecordMemoryRecallTracePublishesRecordedNotificationThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	hub := &recordingMemoryRecallAsyncHub{}
	trace := &memorymodel.MemoryRecallTrace{
		ID:                 "trace-recorded-async",
		WorkspaceID:        "ws-trace-recorded-async",
		RunID:              "run-trace-recorded-async",
		Stage:              "activation",
		Status:             "selected",
		RetrievedMemoryIDs: []common.MemoryID{"mem-selected"},
		OccurredAt:         "2026-06-21T00:00:00Z",
	}

	if err := RecordMemoryRecallTrace(ctx, hub, trace); err != nil {
		t.Fatalf("RecordMemoryRecallTrace: %v", err)
	}
	if len(hub.sync) != 1 || hub.sync[0].Topic != memoryevents.TopicMemoryRecallTraceCommandUpsert {
		t.Fatalf("sync publishes = %+v, want one trace upsert command", hub.sync)
	}
	if len(hub.async) != 1 || hub.async[0].Topic != memoryevents.TopicMemoryRecallRecorded {
		t.Fatalf("async publishes = %+v, want one memory recall recorded notification", hub.async)
	}
	if hub.async[0].WorkspaceID != string(trace.WorkspaceID) || hub.async[0].RunID != string(trace.RunID) {
		t.Fatalf("async scope = workspace:%q run:%q", hub.async[0].WorkspaceID, hub.async[0].RunID)
	}
}

type recordingMemoryRecallAsyncHub struct {
	sync  []events.Envelope
	async []events.Envelope
}

func (h *recordingMemoryRecallAsyncHub) Publish(_ context.Context, envelope events.Envelope) error {
	h.sync = append(h.sync, envelope)
	return events.Response(envelope)
}

func (h *recordingMemoryRecallAsyncHub) PublishAsync(_ context.Context, envelope events.Envelope) error {
	h.async = append(h.async, envelope)
	return nil
}

func (h *recordingMemoryRecallAsyncHub) Subscribe(string, events.Handler) func() {
	return func() {}
}

package ownercommandports

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

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
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHub(rawHub)
	var captured memoryevents.UpsertMemoryRecallTraceCommand
	subscribeMemoryCommand(t, rawHub, memoryevents.TopicMemoryRecallTraceCommandUpsert, func(cmd memoryevents.UpsertMemoryRecallTraceCommand, result magicevent.Result) {
		captured = cmd
		result.Set(memoryevents.MemoryCommandResult{}, nil)
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

	if captured.TraceID != trace.ID || captured.WorkspaceID != trace.WorkspaceID || captured.RunID != trace.RunID || captured.TypeQuota["episodic"] != 2 {
		t.Fatalf("command = %+v, want populated trace command", captured)
	}
	if len(captured.RetrievedMemoryHits) != 1 || captured.RetrievedMemoryHits[0].ID != "mem-hit" {
		t.Fatalf("retrieved hits = %+v, want mem-hit", captured.RetrievedMemoryHits)
	}
}

func TestRecordMemoryRecallTracePublishesRecordedNotificationThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHub(rawHub)
	subscribeMemoryCommand(t, rawHub, memoryevents.TopicMemoryRecallTraceCommandUpsert, func(_ memoryevents.UpsertMemoryRecallTraceCommand, result magicevent.Result) {
		result.Set(memoryevents.MemoryCommandResult{}, nil)
	})
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
	published := hub.Published()
	if len(published) != 1 || published[0].Topic != memoryevents.TopicMemoryRecallRecorded {
		t.Fatalf("published = %+v, want one memory recall recorded notification", published)
	}
	if published[0].WorkspaceID != string(trace.WorkspaceID) || published[0].RunID != string(trace.RunID) {
		t.Fatalf("notification scope = workspace:%q run:%q", published[0].WorkspaceID, published[0].RunID)
	}
}

func subscribeMemoryCommand[Command any](t *testing.T, hub magicevent.Hub, topic string, handle func(Command, magicevent.Result)) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(memoryevents.CommandDestination, hub)
	observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(Command)
		if !ok {
			t.Fatalf("event %s command type = %T", topic, ev.Data())
		}
		handle(cmd, result)
	})
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

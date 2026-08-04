package server

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

var serverTestEventSequence atomic.Uint64

func mustServerEventPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("event payload: %v", err)
	}
	return payload
}

func publishServerTestEventRecord(ctx context.Context, hub magicevent.Hub, envelope streamEvent) error {
	if envelope.ID == "" {
		envelope.ID = fmt.Sprintf("eventstream-test-%d", serverTestEventSequence.Add(1))
	}
	if envelope.Version == "" {
		envelope.Version = recordpayload.WorkorchEventVersion
	}
	if envelope.Revision == 0 {
		envelope.Revision = 1
	}
	data, err := eventstreamevents.EventRecordDataFromModel(&eventstreammodel.EventRecord{
		ID:           common.EventRecordID(envelope.ID),
		Version:      envelope.Version,
		Topic:        envelope.Topic,
		OccurredAt:   envelope.OccurredAt,
		SourceModule: envelope.SourceModule,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  common.WorkspaceID(envelope.WorkspaceID),
		RunID:        common.RunID(envelope.RunID),
		StepID:       common.StepID(envelope.StepID),
		Revision:     envelope.Revision,
		Payload:      mustServerEventPayloadValue(envelope.Payload),
	})
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamRecordAppended, "blocks/eventstream", eventstreamevents.NotificationDestination, magicevent.NewHeader(), ctx, eventstreamevents.EventRecordAppendedData{Record: data})
	result := hub.Send(event)
	if result == nil {
		return fmt.Errorf("record appended event returned no result")
	}
	if resultErr := result.Error(); resultErr != nil {
		if strings.Contains(resultErr.Message, "missing observer") {
			return nil
		}
		return resultErr
	}
	return nil
}

func mustServerEventPayloadValue(input map[string]any) workorcheventpayload.Payload {
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		panic(err)
	}
	return payload
}

func TestNormalizeEventStreamViewDefaultsToProjection(t *testing.T) {
	if got := normalizeEventStreamView(""); got != eventStreamViewProjection {
		t.Fatalf("default stream view = %q, want projection", got)
	}
	if got := normalizeEventStreamView("full"); got != eventStreamViewFull {
		t.Fatalf("full stream view = %q, want full", got)
	}
	if got := normalizeEventStreamView("unknown"); got != eventStreamViewProjection {
		t.Fatalf("unknown stream view = %q, want projection", got)
	}
}

func TestServerMetricsObserveRunStateIsExactlyOnceAndMonotonic(t *testing.T) {
	metrics := &serverMetrics{}
	metrics.observeRunState("ws-metrics", "run-metrics", string(common.RunStateActive), 7, 1)
	metrics.observeRunState("ws-metrics", "run-metrics", string(common.RunStateActive), 8, 3)
	// A replayed terminal notification must neither duplicate the outcome nor
	// regress the persisted repair snapshot after activeRun is released.
	metrics.observeRunState("ws-metrics", "run-metrics", string(common.RunStateFailed), 9, 3)
	metrics.observeRunState("ws-metrics", "run-metrics", string(common.RunStateFailed), 9, 1)
	if got := metrics.runFailed.Load(); got != 1 {
		t.Fatalf("failed outcomes = %d, want 1", got)
	}
	if got := metrics.convergenceRepairAttempts.Load(); got != 3 {
		t.Fatalf("repair attempt delta = %d, want 3", got)
	}
	metrics.observeRunState("ws-metrics", "run-canceled", string(common.RunStateCancelled), 4, 0)
	metrics.observeRunState("ws-metrics", "run-superseded", string(common.RunStateSuperseded), 5, 0)
	metrics.observeRunState("ws-metrics", "run-completed", string(common.RunStateCompleted), 6, 0)
	if metrics.runCanceled.Load() != 1 || metrics.runRejected.Load() != 1 || metrics.runSucceeded.Load() != 1 {
		t.Fatalf("terminal mappings = canceled:%d rejected:%d succeeded:%d", metrics.runCanceled.Load(), metrics.runRejected.Load(), metrics.runSucceeded.Load())
	}
}

func TestServerRunMetricsConsumeTypedRunNotificationNotEventstreamPayload(t *testing.T) {
	state := &serverState{metrics: &serverMetrics{}}
	state.RecordEventRecordAppended(context.Background(), eventstreamevents.EventRecordData{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-metrics",
		RunID:       "run-metrics",
		Payload: mustServerEventPayload(t, map[string]any{
			recordpayload.PayloadKeyState:                string(common.RunStateFailed),
			recordpayload.PayloadKeyRuntimeStateRevision: 9,
		}),
	})
	if got := state.metrics.runFailed.Load(); got != 0 {
		t.Fatalf("eventstream payload changed terminal metric to %d, want 0", got)
	}
	if got := state.metrics.convergenceRepairAttempts.Load(); got != 0 {
		t.Fatalf("eventstream payload changed repair metric to %d, want 0", got)
	}

	state.RecordRunStateChanged(context.Background(), runevents.RunStateChangedData{
		WorkspaceID: "ws-metrics", RunID: "run-metrics", State: common.RunStateFailed,
		RuntimeStateRevision: 9, RuntimeStateRef: "runtime-state/ws-metrics/run-metrics/9",
	})
	if got := state.metrics.runFailed.Load(); got != 1 {
		t.Fatalf("typed run notification terminal metric = %d, want 1", got)
	}
	if got := state.metrics.convergenceRepairAttempts.Load(); got != 0 {
		t.Fatalf("run notification must not carry orchestrator repair state, got %d", got)
	}
}

func TestSubscribeServerStreamEventsReceivesProjectionAcrossHubWrappers(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	received := make(chan streamEvent, 1)
	unsubscribers := subscribeServerStreamEvents(rawHub, eventStreamViewFull, func(_ context.Context, envelope streamEvent) error {
		received <- envelope
		return nil
	})
	defer unsubscribeAll(unsubscribers)

	rawHub.Post(magicevent.NewEventWithContext(
		workorchdevents.TopicWorkspaceProjectionChanged,
		"workorchd/projection",
		workorchdevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		workorchdevents.WorkspaceProjectionChangedData{WorkspaceID: "ws-projection", ChangeKind: "detail"},
	))

	select {
	case envelope := <-received:
		if envelope.Topic != workorchdevents.TopicWorkspaceProjectionChanged || envelope.WorkspaceID != "ws-projection" {
			t.Fatalf("envelope = %+v, want projection event for ws-projection", envelope)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for projection event across hub wrappers")
	}
}

func TestSubscribeServerStreamEventsReceivesOwnerEventAcrossHubWrappers(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	received := make(chan streamEvent, 1)
	unsubscribers := subscribeServerStreamEvents(rawHub, eventStreamViewFull, func(_ context.Context, envelope streamEvent) error {
		received <- envelope
		return nil
	})
	defer unsubscribeAll(unsubscribers)

	if err := publishServerTestEventRecord(ctx, rawHub, streamEvent{
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-owner",
		Payload:     recordpayload.WorkspacePayloadData{Title: "owner workspace", DeliverableDirectory: "/tmp/ws-owner", State: common.WorkspaceStateActive}.EventPayload().Map(),
	}); err != nil {
		t.Fatalf("publish owner event: %v", err)
	}

	select {
	case envelope := <-received:
		if envelope.Topic != workspaceevents.TopicWorkspaceCreated || envelope.WorkspaceID != "ws-owner" {
			t.Fatalf("envelope = %+v, want workspace created event for ws-owner", envelope)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for owner event across hub wrappers")
	}
}

func TestSubscribeServerStreamEventsReceivesTicketCreatedAcrossHubWrappers(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	received := make(chan streamEvent, 1)
	unsubscribers := subscribeServerStreamEvents(rawHub, eventStreamViewFull, func(_ context.Context, envelope streamEvent) error {
		received <- envelope
		return nil
	})
	defer unsubscribeAll(unsubscribers)

	if err := publishServerTestEventRecord(ctx, rawHub, streamEvent{
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       decisionevents.TopicTicketCreated,
		WorkspaceID: "ws-ticket-created",
		RunID:       "run-ticket-created",
		StepID:      "step-ticket-created",
		SubjectType: "decision_ticket",
		SubjectID:   "ticket-created",
		Payload:     recordpayload.DecisionTicketPayloadData{Reason: "provider unavailable", State: common.DecisionTicketStateOpen}.EventPayload().Map(),
	}); err != nil {
		t.Fatalf("publish ticket created event: %v", err)
	}

	select {
	case envelope := <-received:
		if envelope.Topic != decisionevents.TopicTicketCreated || envelope.WorkspaceID != "ws-ticket-created" {
			t.Fatalf("envelope = %+v, want ticket created event for ws-ticket-created", envelope)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for ticket created event across hub wrappers")
	}
}

func TestSubscribeServerStreamEventsProjectionViewIgnoresOwnerEvents(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	received := make(chan streamEvent, 1)
	unsubscribers := subscribeServerStreamEvents(rawHub, eventStreamViewProjection, func(_ context.Context, envelope streamEvent) error {
		received <- envelope
		return nil
	})
	defer unsubscribeAll(unsubscribers)

	if err := publishServerTestEventRecord(ctx, rawHub, streamEvent{
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-owner",
		Payload:     recordpayload.WorkspacePayloadData{Title: "owner workspace", DeliverableDirectory: "/tmp/ws-owner", State: common.WorkspaceStateActive}.EventPayload().Map(),
	}); err != nil {
		t.Fatalf("publish owner event: %v", err)
	}

	select {
	case envelope := <-received:
		t.Fatalf("unexpected projection stream envelope = %+v", envelope)
	case <-time.After(150 * time.Millisecond):
	}
}

// These test-only helpers model the Biz subscriptions. Production server code
// receives typed callbacks from Biz and never creates EventHub observers.
func subscribeServerStreamEvents(hub magicevent.Hub, view string, handler func(context.Context, streamEvent) error) []func() {
	if hub == nil || handler == nil {
		return nil
	}
	projection := magicevent.NewSimpleObserver(workorchdevents.NotificationDestination, hub)
	projection.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData); ok {
			_ = handler(event.Context(), workspaceProjectionStreamEvent(data))
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	unsubscribers := []func(){func() { projection.Unsubscribe(workorchdevents.TopicWorkspaceProjectionChanged) }}
	if isProjectionEventStreamView(view) {
		return unsubscribers
	}
	records := magicevent.NewSimpleObserver(eventstreamevents.NotificationDestination, hub)
	records.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(eventstreamevents.EventRecordAppendedData); ok {
			if envelope, valid := eventRecordStreamEvent(data.Record); valid {
				_ = handler(event.Context(), envelope)
			}
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	return append(unsubscribers, func() { records.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended) })
}

func unsubscribeAll(unsubscribers []func()) {
	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}

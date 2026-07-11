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
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

var serverTestEventSequence atomic.Uint64

func publishServerTestEventRecord(ctx context.Context, hub magicevent.Hub, envelope streamEvent) error {
	if envelope.ID == "" {
		envelope.ID = fmt.Sprintf("eventstream-test-%d", serverTestEventSequence.Add(1))
	}
	if envelope.Version == "" {
		envelope.Version = events.WorkorchEventVersion
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
		Payload:      envelope.Payload,
	})
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamRecordAppended, "kernel/eventstream", eventstreamevents.NotificationDestination, magicevent.NewHeader(), ctx, eventstreamevents.EventRecordAppendedData{Record: data})
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
		Version:     events.WorkorchEventVersion,
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-owner",
		Payload:     events.WorkspacePayload("owner workspace", "", "/tmp/ws-owner", "", common.WorkspaceStateActive, ""),
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

func TestSubscribeServerStreamEventsReceivesTicketUpdatedAcrossHubWrappers(t *testing.T) {
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
		Version:     events.WorkorchEventVersion,
		Topic:       decisionevents.TopicTicketUpdated,
		WorkspaceID: "ws-ticket-updated",
		RunID:       "run-ticket-updated",
		StepID:      "step-ticket-updated",
		SubjectType: "decision_ticket",
		SubjectID:   "ticket-updated",
		Payload:     events.DecisionTicketPayload("provider unavailable", common.DecisionTicketStateOpen),
	}); err != nil {
		t.Fatalf("publish ticket updated event: %v", err)
	}

	select {
	case envelope := <-received:
		if envelope.Topic != decisionevents.TopicTicketUpdated || envelope.WorkspaceID != "ws-ticket-updated" {
			t.Fatalf("envelope = %+v, want ticket updated event for ws-ticket-updated", envelope)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for ticket updated event across hub wrappers")
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
		Version:     events.WorkorchEventVersion,
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-owner",
		Payload:     events.WorkspacePayload("owner workspace", "", "/tmp/ws-owner", "", common.WorkspaceStateActive, ""),
	}); err != nil {
		t.Fatalf("publish owner event: %v", err)
	}

	select {
	case envelope := <-received:
		t.Fatalf("unexpected projection stream envelope = %+v", envelope)
	case <-time.After(150 * time.Millisecond):
	}
}

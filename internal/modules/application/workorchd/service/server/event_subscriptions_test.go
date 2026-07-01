package server

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

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

	subscriber := events.NewMagicHubWithHubOwner(rawHub, "workorchd/sse")
	publisher := events.NewMagicHubWithHubOwner(rawHub, "workorchd/projection")
	received := make(chan events.Envelope, 1)
	unsubscribers := subscribeServerStreamEvents(subscriber, eventStreamViewFull, func(_ context.Context, envelope events.Envelope) error {
		received <- envelope
		return nil
	})
	defer unsubscribeAll(unsubscribers)

	if err := publisher.Publish(ctx, events.Envelope{
		Topic:       workorchdevents.TopicWorkspaceProjectionChanged,
		WorkspaceID: "ws-projection",
		Payload:     map[string]any{"changeKind": "detail"},
	}); err != nil {
		t.Fatalf("publish projection event: %v", err)
	}

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

	subscriber := events.NewMagicHubWithHubOwner(rawHub, "workorchd/sse")
	publisher := events.NewMagicHubWithHubOwner(rawHub, "kernel/workspace")
	received := make(chan events.Envelope, 1)
	unsubscribers := subscribeServerStreamEvents(subscriber, eventStreamViewFull, func(_ context.Context, envelope events.Envelope) error {
		received <- envelope
		return nil
	})
	defer unsubscribeAll(unsubscribers)

	if err := publisher.Publish(ctx, events.Envelope{
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

	subscriber := events.NewMagicHubWithHubOwner(rawHub, "workorchd/sse")
	publisher := events.NewMagicHubWithHubOwner(rawHub, "kernel/decision")
	received := make(chan events.Envelope, 1)
	unsubscribers := subscribeServerStreamEvents(subscriber, eventStreamViewFull, func(_ context.Context, envelope events.Envelope) error {
		received <- envelope
		return nil
	})
	defer unsubscribeAll(unsubscribers)

	if err := publisher.Publish(ctx, events.Envelope{
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

	subscriber := events.NewMagicHubWithHubOwner(rawHub, "workorchd/sse")
	publisher := events.NewMagicHubWithHubOwner(rawHub, "kernel/workspace")
	received := make(chan events.Envelope, 1)
	unsubscribers := subscribeServerStreamEvents(subscriber, eventStreamViewProjection, func(_ context.Context, envelope events.Envelope) error {
		received <- envelope
		return nil
	})
	defer unsubscribeAll(unsubscribers)

	if err := publisher.Publish(ctx, events.Envelope{
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

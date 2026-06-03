package appservice

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestEventHubActionPortReturnsInvalidArgumentWhenHubOrRequiredInputMissing(t *testing.T) {
	port := newEventHubActionPort(nil)
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "workspace ensure no hub", run: func() error { return port.EnsureWorkspace(context.Background(), "ws", "title") }},
		{name: "run ensure no hub", run: func() error { return port.EnsureRun(context.Background(), "run", "ws") }},
		{name: "run started no hub", run: func() error { return port.EnsureRunStarted(context.Background(), "run") }},
		{name: "decision ticket no hub", run: func() error {
			return port.EnsureTicket(context.Background(), "ticket", "ws", "run", "run/manual-gate", "reason")
		}},
		{name: "approval ensure no hub", run: func() error {
			return port.EnsureApproval(context.Background(), "approval", "ws", "run", "run/manual-approval", "reason")
		}},
		{name: "workspace ensure missing id", run: func() error {
			return newEventHubActionPort(events.NewMemoryHub()).EnsureWorkspace(context.Background(), "", "title")
		}},
		{name: "run ensure missing workspace", run: func() error {
			return newEventHubActionPort(events.NewMemoryHub()).EnsureRun(context.Background(), "run", "")
		}},
		{name: "run blocked decision", run: func() error {
			return newEventHubActionPort(events.NewMemoryHub()).EnsureRunBlockedOnDecision(context.Background(), "run", "")
		}},
		{name: "run blocked approval", run: func() error {
			return newEventHubActionPort(events.NewMemoryHub()).EnsureRunBlockedOnApproval(context.Background(), "", "approval")
		}},
		{name: "run resumed missing id", run: func() error {
			return newEventHubActionPort(events.NewMemoryHub()).EnsureRunResumed(context.Background(), "")
		}},
		{name: "decision resolved missing resolution", run: func() error {
			return newEventHubActionPort(events.NewMemoryHub()).EnsureResolvedTicket(context.Background(), "", "ticket", "provide_missing_info", nil)
		}},
		{name: "approval approved missing id", run: func() error {
			return newEventHubActionPort(events.NewMemoryHub()).EnsureApprovalApproved(context.Background(), "")
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}

func TestEventHubActionPortPublishesOwnerCommands(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	port := newEventHubActionPort(hub)
	wantTopics := []string{
		events.TopicWorkspaceCommandEnsure,
		events.TopicRunCommandEnsure,
		events.TopicRunCommandStart,
		events.TopicOrchestratorCommandBlockDecision,
		events.TopicOrchestratorCommandBlockApproval,
		events.TopicRunCommandEnsureResumed,
		events.TopicDecisionCommandEnsureTicket,
		events.TopicDecisionCommandEnsureResolved,
		events.TopicApprovalCommandEnsure,
		events.TopicApprovalCommandEnsureApproved,
		events.TopicApprovalCommandEnsureRejected,
	}
	for _, topic := range wantTopics {
		topic := topic
		hub.Subscribe(topic, func(_ context.Context, envelope events.Envelope) error {
			return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: envelope.Payload})
		})
	}

	if err := port.EnsureWorkspace(ctx, "ws-action-port", "workspace action port"); err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := port.EnsureRun(ctx, "run-action-port", "ws-action-port"); err != nil {
		t.Fatalf("EnsureRun: %v", err)
	}
	if err := port.EnsureRunStarted(ctx, "run-action-port"); err != nil {
		t.Fatalf("EnsureRunStarted: %v", err)
	}
	if err := port.EnsureRunBlockedOnDecision(ctx, "run-action-port", "ticket-action-port"); err != nil {
		t.Fatalf("EnsureRunBlockedOnDecision: %v", err)
	}
	if err := port.EnsureRunBlockedOnApproval(ctx, "run-action-port", "approval-action-port"); err != nil {
		t.Fatalf("EnsureRunBlockedOnApproval: %v", err)
	}
	if err := port.EnsureRunResumed(ctx, "run-action-port"); err != nil {
		t.Fatalf("EnsureRunResumed: %v", err)
	}
	payload := map[string]any{"operator": "action-port"}
	if err := port.EnsureTicket(ctx, "ticket-action-port", "ws-action-port", "run-action-port", "run-action-port/manual-gate", "manual decision"); err != nil {
		t.Fatalf("EnsureTicket: %v", err)
	}
	if err := port.EnsureResolvedTicket(ctx, "resolution-action-port", "ticket-action-port", "provide_missing_info", payload); err != nil {
		t.Fatalf("EnsureResolvedTicket: %v", err)
	}
	if err := port.EnsureApproval(ctx, "approval-action-port", "ws-action-port", "run-action-port", "run-action-port/manual-approval", "manual approval"); err != nil {
		t.Fatalf("EnsureApproval: %v", err)
	}
	if err := port.EnsureApprovalApproved(ctx, "approval-action-port"); err != nil {
		t.Fatalf("EnsureApprovalApproved: %v", err)
	}
	if err := port.EnsureApprovalRejected(ctx, "approval-action-port"); err != nil {
		t.Fatalf("EnsureApprovalRejected: %v", err)
	}

	published := hub.Published()
	if len(published) != len(wantTopics) {
		t.Fatalf("published events = %d, want %d", len(published), len(wantTopics))
	}
	for idx, topic := range wantTopics {
		if published[idx].Topic != topic {
			t.Fatalf("event[%d].Topic = %s, want %s", idx, published[idx].Topic, topic)
		}
		if published[idx].SourceModule != "workorchd/appservice/runtime-actions" {
			t.Fatalf("event[%d].SourceModule = %s", idx, published[idx].SourceModule)
		}
	}
	if published[0].Payload[events.PayloadKeyWorkspaceID] != "ws-action-port" || published[0].Payload[events.PayloadKeyTitle] != "workspace action port" {
		t.Fatalf("workspace ensure payload = %#v", published[0].Payload)
	}
	if published[1].Payload[events.PayloadKeyRunID] != "run-action-port" || published[1].Payload[events.PayloadKeyWorkspaceID] != "ws-action-port" {
		t.Fatalf("run ensure payload = %#v", published[1].Payload)
	}
	if published[6].Payload[events.PayloadKeyBlockingTicketID] != "ticket-action-port" || published[6].Payload[events.PayloadKeyWorkspaceID] != "ws-action-port" || published[6].Payload[events.PayloadKeyRunID] != "run-action-port" {
		t.Fatalf("decision ensure payload = %#v", published[6].Payload)
	}
	if published[8].Payload[events.PayloadKeyApprovalID] != "approval-action-port" || published[8].Payload[events.PayloadKeyWorkspaceID] != "ws-action-port" || published[8].Payload[events.PayloadKeyRunID] != "run-action-port" {
		t.Fatalf("approval ensure payload = %#v", published[8].Payload)
	}
}

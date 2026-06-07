package decision

import (
	"context"
	"errors"
	"testing"

	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestDecisionTicketCommandGetHonorsScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-scoped-get")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-ticket-owner-a",
		RunID:       "run-ticket-owner-a",
		StepID:      "step-ticket-owner-a",
		State:       common.DecisionTicketStateOpen,
		Reason:      "decision scope test",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	err := hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicDecisionCommandGetTicket,
		WorkspaceID: "ws-ticket-owner-b",
		RunID:       "run-ticket-owner-a",
		StepID:      "step-ticket-owner-a",
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyWorkspaceID:      "ws-ticket-owner-b",
			events.PayloadKeyRunID:            "run-ticket-owner-a",
			events.PayloadKeyStepID:           "step-ticket-owner-a",
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicDecisionCommandGetTicket,
		WorkspaceID: "ws-ticket-owner-a",
		RunID:       "run-ticket-owner-a",
		StepID:      "step-ticket-owner-a",
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyWorkspaceID:      "ws-ticket-owner-a",
			events.PayloadKeyRunID:            "run-ticket-owner-a",
			events.PayloadKeyStepID:           "step-ticket-owner-a",
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("scoped get err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)
	if !ok || item == nil || item.WorkspaceID != "ws-ticket-owner-a" || item.ID != ticketID {
		t.Fatalf("response ticket = %+v, want scoped ticket", response.Payload[events.PayloadKeyTicket])
	}
}

func TestDecisionTicketCommandListByRunHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	runID := common.RunID("run-shared-ticket")
	for _, item := range []*decisionmodel.Ticket{
		{
			ID:          "ticket-list-a",
			WorkspaceID: "ws-ticket-list-a",
			RunID:       runID,
			State:       common.DecisionTicketStateOpen,
			Reason:      "decision list scope test",
		},
		{
			ID:          "ticket-list-b",
			WorkspaceID: "ws-ticket-list-b",
			RunID:       runID,
			State:       common.DecisionTicketStateOpen,
			Reason:      "decision list scope test",
		},
	} {
		if err := tickets.Create(ctx, item); err != nil {
			t.Fatalf("create ticket %s: %v", item.ID, err)
		}
	}

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	err := hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicDecisionCommandListTicketsByRun,
		WorkspaceID: "ws-ticket-list-a",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-ticket-list-a",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("list by run err = %v, want response", err)
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok || len(items) != 1 || items[0].WorkspaceID != "ws-ticket-list-a" {
		t.Fatalf("response tickets = %+v, want one scoped ticket", response.Payload[events.PayloadKeyTickets])
	}
}

func TestDecisionMutationCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-scoped-mutation")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-ticket-mutate-a",
		RunID:       "run-ticket-mutate-a",
		State:       common.DecisionTicketStateOpen,
		Reason:      "decision mutation scope test",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	err := hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicDecisionCommandEnsureResolved,
		WorkspaceID: "ws-ticket-mutate-b",
		RunID:       "run-ticket-mutate-a",
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyResolutionID:     "resolution-scoped-mutation",
			events.PayloadKeyType:             "provide_missing_info",
			events.PayloadKeyWorkspaceID:      "ws-ticket-mutate-b",
			events.PayloadKeyRunID:            "run-ticket-mutate-a",
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace resolve err = %v, want not found", err)
	}
	item, err := tickets.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if item.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %s, want open", item.State)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicDecisionCommandDeleteTicket,
		WorkspaceID: "ws-ticket-mutate-b",
		RunID:       "run-ticket-mutate-a",
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyWorkspaceID:      "ws-ticket-mutate-b",
			events.PayloadKeyRunID:            "run-ticket-mutate-a",
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace delete err = %v, want not found", err)
	}
	if _, err := tickets.Get(ctx, ticketID); err != nil {
		t.Fatalf("ticket should remain after wrong-scope delete: %v", err)
	}
}

func TestDecisionCreateResolutionCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	resolutions := memory.NewDecisionResolutionRepository()
	module := New()
	if err := module.Configure(Dependencies{
		Tickets:     memory.NewDecisionTicketRepository(),
		Resolutions: resolutions,
		EventHub:    hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	ticketID := common.TicketID("ticket-resolution-contract")
	err := hub.Publish(ctx, events.Envelope{
		Topic:       events.TopicDecisionCommandCreateResolution,
		SubjectType: "decision_resolution",
		SubjectID:   "resolution-contract",
		Payload: map[string]any{
			events.PayloadKeyCommand: decisioncontracts.CreateResolutionCommand{
				ResolutionID: "resolution-contract",
				TicketID:     ticketID,
				Type:         "provide_missing_info",
				Payload:      map[string]any{"answer": "ok"},
			},
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("create resolution err = %v, want response", err)
	}
	item, err := resolutions.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	if item.ID != "resolution-contract" || item.Type != "provide_missing_info" || item.Payload["answer"] != "ok" {
		t.Fatalf("resolution = %+v, want command fields", item)
	}
}

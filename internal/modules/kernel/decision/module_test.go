package decision

import (
	"context"
	"errors"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.Service() != nil {
		t.Fatal("new module service is non-nil before Configure")
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}

	if err := module.Configure(decisionModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.Service() == nil {
		t.Fatal("service is nil after Configure")
	}
	if module.bizPtr == nil ||
		module.servicePtr == nil ||
		module.eventHub == nil ||
		module.tickets == nil ||
		module.resolutions == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.unsub)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe decision commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.unsub) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.unsub), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.Service() != nil ||
		module.bizPtr != nil ||
		module.servicePtr != nil ||
		module.eventHub != nil ||
		module.tickets != nil ||
		module.resolutions != nil ||
		len(module.unsub) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := decisionModuleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing tickets", deps: func() Dependencies { d := base; d.Tickets = nil; return d }()},
		{name: "missing resolutions", deps: func() Dependencies { d := base; d.Resolutions = nil; return d }()},
		{name: "missing event hub", deps: func() Dependencies { d := base; d.EventHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

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
		Topic:       decisionevents.TopicDecisionCommandGetTicket,
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
		Topic:       decisionevents.TopicDecisionCommandGetTicket,
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
		Topic:       decisionevents.TopicDecisionCommandListTicketsByRun,
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
		Topic:       decisionevents.TopicDecisionCommandEnsureResolved,
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
		Topic:       decisionevents.TopicDecisionCommandDeleteTicket,
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

func TestDecisionResolveCommandWithoutScopeResolvesUniqueOpenTicket(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-unscoped-unique")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-ticket-unscoped-unique",
		RunID:       "run-ticket-unscoped-unique",
		StepID:      "step-ticket-unscoped-unique",
		State:       common.DecisionTicketStateOpen,
		Reason:      "decision unscoped unique test",
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
		Topic: decisionevents.TopicDecisionCommandEnsureResolved,
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyResolutionID:     "resolution-unscoped-unique",
			events.PayloadKeyType:             "provide_missing_info",
			events.PayloadKeyPayload:          map[string]any{"answer": "ok"},
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("unscoped resolve err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)
	if !ok || item == nil || item.ID != ticketID || item.WorkspaceID != "ws-ticket-unscoped-unique" || item.State != common.DecisionTicketStateResolved {
		t.Fatalf("response ticket = %+v, want resolved unique ticket", response.Payload[events.PayloadKeyTicket])
	}
	resolution, err := resolutions.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	if resolution.ID != "resolution-unscoped-unique" || resolution.Type != "provide_missing_info" {
		t.Fatalf("resolution = %+v, want unscoped unique resolution", resolution)
	}
}

func TestDecisionResolveCommandWithoutScopeConflictsOnDuplicateOpenTickets(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-unscoped-duplicate")
	for _, item := range []*decisionmodel.Ticket{
		{
			ID:          ticketID,
			WorkspaceID: "ws-ticket-unscoped-duplicate-a",
			RunID:       "run-ticket-unscoped-duplicate-a",
			State:       common.DecisionTicketStateOpen,
			Reason:      "decision unscoped duplicate test",
		},
		{
			ID:          ticketID,
			WorkspaceID: "ws-ticket-unscoped-duplicate-b",
			RunID:       "run-ticket-unscoped-duplicate-b",
			State:       common.DecisionTicketStateOpen,
			Reason:      "decision unscoped duplicate test",
		},
	} {
		if err := tickets.Create(ctx, item); err != nil {
			t.Fatalf("create ticket %s/%s: %v", item.WorkspaceID, item.ID, err)
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
		Topic: decisionevents.TopicDecisionCommandEnsureResolved,
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyResolutionID:     "resolution-unscoped-duplicate",
			events.PayloadKeyType:             "provide_missing_info",
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate resolve err = %v, want conflict", err)
	}
	if _, err := resolutions.GetByTicketID(ctx, ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("duplicate resolve resolution err = %v, want not found", err)
	}
	items, err := tickets.List(ctx)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	for _, item := range items {
		if item.ID == ticketID && item.State != common.DecisionTicketStateOpen {
			t.Fatalf("ticket %s state = %s, want open after duplicate conflict", item.WorkspaceID, item.State)
		}
	}
}

func TestDecisionDeleteCommandWithoutScopeDeletesUniqueOpenTicket(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-unscoped-delete")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-ticket-unscoped-delete",
		RunID:       "run-ticket-unscoped-delete",
		State:       common.DecisionTicketStateOpen,
		Reason:      "decision unscoped delete test",
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
		Topic: decisionevents.TopicDecisionCommandDeleteTicket,
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("unscoped delete err = %v, want response", err)
	}
	if _, err := tickets.Get(ctx, ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get deleted ticket err = %v, want not found", err)
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
		Topic:       decisionevents.TopicDecisionCommandCreateResolution,
		SubjectType: "decision_resolution",
		SubjectID:   "resolution-contract",
		Payload: map[string]any{
			events.PayloadKeyCommand: decisioncontracts.CreateResolutionCommand{
				ResolutionID:       "resolution-contract",
				TicketID:           ticketID,
				Type:               "provide_missing_info",
				Payload:            map[string]any{"answer": "ok"},
				ContinuationAction: "resume",
				ResumeRunID:        "run-current",
				SuccessorRunID:     "run-next",
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
	if item.ContinuationAction != "resume" || item.ResumeRunID != "run-current" || item.SuccessorRunID != "run-next" {
		t.Fatalf("resolution refs = %+v, want command recovery refs", item)
	}
}

func TestDecisionResolutionCommandGetHonorsTicketScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-resolution-scope")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-resolution-owner-a",
		RunID:       "run-resolution-owner-a",
		StepID:      "step-resolution-owner-a",
		State:       common.DecisionTicketStateResolved,
		Reason:      "decision resolution scope test",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := resolutions.Create(common.ContextWithRuntimeWorkspaceID(ctx, "ws-resolution-owner-a"), &decisionmodel.Resolution{
		ID:       "resolution-owner-a",
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"answer": "ok"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       decisionevents.TopicDecisionCommandGetResolution,
		WorkspaceID: "ws-resolution-owner-b",
		RunID:       "run-resolution-owner-a",
		StepID:      "step-resolution-owner-a",
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyWorkspaceID:      "ws-resolution-owner-b",
			events.PayloadKeyRunID:            "run-resolution-owner-a",
			events.PayloadKeyStepID:           "step-resolution-owner-a",
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get resolution err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       decisionevents.TopicDecisionCommandGetResolution,
		WorkspaceID: "ws-resolution-owner-a",
		RunID:       "run-resolution-owner-a",
		StepID:      "step-resolution-owner-a",
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyWorkspaceID:      "ws-resolution-owner-a",
			events.PayloadKeyRunID:            "run-resolution-owner-a",
			events.PayloadKeyStepID:           "step-resolution-owner-a",
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("scoped get resolution err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeyResolution].(*decisionmodel.Resolution)
	if !ok || item == nil || item.ID != "resolution-owner-a" || item.TicketID != ticketID {
		t.Fatalf("response resolution = %+v, want scoped resolution", response.Payload[events.PayloadKeyResolution])
	}
}

func TestDecisionResolutionCommandWithoutScopeConflictsOnDuplicateOpenTickets(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-resolution-duplicate")
	for _, item := range []*decisionmodel.Ticket{
		{
			ID:          ticketID,
			WorkspaceID: "ws-resolution-duplicate-a",
			RunID:       "run-resolution-duplicate-a",
			State:       common.DecisionTicketStateOpen,
			Reason:      "decision resolution duplicate test",
		},
		{
			ID:          ticketID,
			WorkspaceID: "ws-resolution-duplicate-b",
			RunID:       "run-resolution-duplicate-b",
			State:       common.DecisionTicketStateOpen,
			Reason:      "decision resolution duplicate test",
		},
	} {
		if err := tickets.Create(ctx, item); err != nil {
			t.Fatalf("create ticket %s/%s: %v", item.WorkspaceID, item.ID, err)
		}
	}

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic: decisionevents.TopicDecisionCommandGetResolution,
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate get resolution err = %v, want conflict", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic: decisionevents.TopicDecisionCommandDeleteResolution,
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate delete resolution err = %v, want conflict", err)
	}
}

func TestDecisionResolutionDeleteWithoutScopeDeletesUniqueTicketResolution(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-resolution-delete-unique")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-resolution-delete-unique",
		RunID:       "run-resolution-delete-unique",
		State:       common.DecisionTicketStateResolved,
		Reason:      "decision resolution delete unique test",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := resolutions.Create(common.ContextWithRuntimeWorkspaceID(ctx, "ws-resolution-delete-unique"), &decisionmodel.Resolution{
		ID:       "resolution-delete-unique",
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"answer": "ok"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic: decisionevents.TopicDecisionCommandDeleteResolution,
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("unscoped delete resolution err = %v, want response", err)
	}
	if _, err := resolutions.GetByTicketID(common.ContextWithRuntimeWorkspaceID(ctx, "ws-resolution-delete-unique"), ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get deleted resolution err = %v, want not found", err)
	}
}

func decisionModuleContractDependencies() Dependencies {
	return Dependencies{
		Tickets:     memory.NewDecisionTicketRepository(),
		Resolutions: memory.NewDecisionResolutionRepository(),
		EventHub:    events.NewMemoryHub(),
	}
}

func TestDecisionUpdateTicketCommandPublishesUpdatedNotification(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-update-notify")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  "ws-ticket-update",
		RunID:        "run-ticket-update",
		StepID:       "step-ticket-update",
		State:        common.DecisionTicketStateResolved,
		Reason:       "provider unavailable",
		DecisionKind: "operator_input",
		Question:     "provide runtime config",
		Context:      map[string]any{"attempt": float64(1)},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	updated := make(chan events.Envelope, 1)
	unsubscribe := hub.Subscribe(decisionevents.TopicTicketUpdated, func(_ context.Context, envelope events.Envelope) error {
		updated <- envelope
		return nil
	})
	t.Cleanup(unsubscribe)

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       decisionevents.TopicDecisionCommandUpdateTicket,
		WorkspaceID: "ws-ticket-update",
		RunID:       "run-ticket-update",
		StepID:      "step-ticket-update",
		Payload: map[string]any{
			events.PayloadKeyCommand: decisioncontracts.UpdateTicketCommand{
				TicketID:     ticketID,
				WorkspaceID:  "ws-ticket-update",
				RunID:        "run-ticket-update",
				StepID:       "step-ticket-update",
				State:        common.DecisionTicketStateOpen,
				Reason:       "provider unavailable",
				DecisionKind: "operator_input",
				Question:     "provide runtime config",
				Context:      map[string]any{"attempt": float64(2)},
			},
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("update ticket err = %v, want response", err)
	}

	item, err := tickets.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if item.State != common.DecisionTicketStateOpen || item.Context["attempt"] != float64(2) {
		t.Fatalf("ticket after update = %+v, want reopened ticket with updated context", item)
	}

	select {
	case envelope := <-updated:
		if envelope.Topic != decisionevents.TopicTicketUpdated {
			t.Fatalf("updated topic = %s, want %s", envelope.Topic, decisionevents.TopicTicketUpdated)
		}
		if envelope.WorkspaceID != "ws-ticket-update" || envelope.RunID != "run-ticket-update" || envelope.StepID != "step-ticket-update" {
			t.Fatalf("updated scope = ws:%s run:%s step:%s", envelope.WorkspaceID, envelope.RunID, envelope.StepID)
		}
		payload, ok := events.DecodeDecisionTicketPayload(envelope.Payload)
		if !ok {
			t.Fatalf("updated payload = %+v, want decision ticket payload", envelope.Payload)
		}
		if payload.State != common.DecisionTicketStateOpen || payload.Reason != "provider unavailable" {
			t.Fatalf("updated payload = %+v, want reopened provider ticket", payload)
		}
	default:
		t.Fatal("ticket update command did not publish ticket updated notification")
	}
}

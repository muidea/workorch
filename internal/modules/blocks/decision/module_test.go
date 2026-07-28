package decision

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustDecisionEventPayload(t *testing.T, input map[string]any) []decisionevents.DecisionEvidenceField {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("DecisionFieldsFromMap: %v", err)
	}
	return payload.Fields
}

func mustDecisionModelPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func sendDecisionTestCommand[Command any](ctx context.Context, rawHub magicevent.Hub, topic string, command Command) (decisionevents.DecisionCommandResult, error) {
	ev := magicevent.NewEventWithContext(topic, "decision/module_test", decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return decisionevents.DecisionCommandResult{}, err.Cause
		}
		return decisionevents.DecisionCommandResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	response, ok := data.(decisionevents.DecisionCommandResult)
	if !ok {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

func subscribeDecisionEvidenceAck(rawHub magicevent.Hub) func() {
	observer := magicevent.NewSimpleObserver(decisionevents.EvidenceCommandDestination, rawHub)
	observer.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(decisionevents.RecordDecisionEvidenceCommand)
		if !ok {
			result.Set(nil, nil)
			return
		}
		result.Set(decisionevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	return func() { observer.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence) }
}

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/decision" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}

	if err := module.Configure(decisionModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
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
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub, BackgroundRoutine: magictask.NewBackgroundRoutine(1)}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandGetTicket, decisionevents.GetTicketCommand{
		WorkspaceID: "ws-ticket-owner-b",
		RunID:       "run-ticket-owner-a",
		StepID:      "step-ticket-owner-a",
		TicketID:    ticketID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	response, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandGetTicket, decisionevents.GetTicketCommand{
		WorkspaceID: "ws-ticket-owner-a",
		RunID:       "run-ticket-owner-a",
		StepID:      "step-ticket-owner-a",
		TicketID:    ticketID,
	})
	if err != nil {
		t.Fatalf("scoped get err = %v, want response", err)
	}
	if response.Ticket == nil || response.Ticket.WorkspaceID != "ws-ticket-owner-a" || response.Ticket.ID != ticketID {
		t.Fatalf("response ticket = %+v, want scoped ticket", response.Ticket)
	}
}

func TestDecisionTicketCommandListByRunHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	response, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandListTicketsByRun, decisionevents.ListTicketsByRunCommand{
		WorkspaceID: "ws-ticket-list-a",
		RunID:       runID,
	})
	if err != nil {
		t.Fatalf("list by run err = %v, want response", err)
	}
	if len(response.Tickets) != 1 || response.Tickets[0].WorkspaceID != "ws-ticket-list-a" {
		t.Fatalf("response tickets = %+v, want one scoped ticket", response.Tickets)
	}
}

func TestDecisionMutationCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.EnsureResolvedCommand{
		TicketID:     ticketID,
		ResolutionID: "resolution-scoped-mutation",
		Type:         "provide_missing_info",
		WorkspaceID:  "ws-ticket-mutate-b",
		RunID:        "run-ticket-mutate-a",
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

	_, err = sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandDeleteTicket, decisionevents.DeleteTicketCommand{
		TicketID:    ticketID,
		WorkspaceID: "ws-ticket-mutate-b",
		RunID:       "run-ticket-mutate-a",
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
	rawHub := magicevent.NewHub(16)
	defer subscribeDecisionEvidenceAck(rawHub)()
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
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	response, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.EnsureResolvedCommand{
		TicketID:     ticketID,
		ResolutionID: "resolution-unscoped-unique",
		Type:         "provide_missing_info",
		Payload:      mustDecisionEventPayload(t, map[string]any{"answer": "ok"}),
	})
	if err != nil {
		t.Fatalf("unscoped resolve err = %v, want response", err)
	}
	if response.Ticket == nil || response.Ticket.ID != ticketID || response.Ticket.WorkspaceID != "ws-ticket-unscoped-unique" || response.Ticket.State != common.DecisionTicketStateResolved {
		t.Fatalf("response ticket = %+v, want resolved unique ticket", response.Ticket)
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
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.EnsureResolvedCommand{
		TicketID:     ticketID,
		ResolutionID: "resolution-unscoped-duplicate",
		Type:         "provide_missing_info",
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
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandDeleteTicket, decisionevents.DeleteTicketCommand{
		TicketID: ticketID,
	})
	if err != nil {
		t.Fatalf("unscoped delete err = %v, want response", err)
	}
	if _, err := tickets.Get(ctx, ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get deleted ticket err = %v, want not found", err)
	}
}

func TestDecisionCreateResolutionCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-resolution-contract")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-resolution-contract",
		RunID:       "run-resolution-contract",
		StepID:      "step-resolution-contract",
		State:       common.DecisionTicketStateOpen,
		Reason:      "resolution contract",
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	module := New()
	if err := module.Configure(Dependencies{
		Tickets:     tickets,
		Resolutions: resolutions,
		EventHub:    rawHub,
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandCreateResolution, decisionevents.CreateResolutionCommand{
		ResolutionID:       "resolution-contract",
		TicketID:           ticketID,
		WorkspaceID:        "ws-resolution-contract",
		RunID:              "run-resolution-contract",
		StepID:             "step-resolution-contract",
		Type:               "provide_missing_info",
		Payload:            mustDecisionEventPayload(t, map[string]any{"answer": "ok"}),
		ContinuationAction: "resume",
		ResumeRunID:        "run-current",
		SuccessorRunID:     "run-next",
	})
	if err != nil {
		t.Fatalf("create resolution err = %v, want response", err)
	}
	item, err := resolutions.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	answer, _ := item.Payload.String("answer")
	if item.ID != "resolution-contract" || item.Type != "provide_missing_info" || answer != "ok" {
		t.Fatalf("resolution = %+v, want command fields", item)
	}
	if item.ContinuationAction != "resume" || item.ResumeRunID != "run-current" || item.SuccessorRunID != "run-next" {
		t.Fatalf("resolution refs = %+v, want command recovery refs", item)
	}
}

func TestDecisionCreateResolutionCommandRequiresTicketScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-resolution-scope-create")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-resolution-create-owner",
		RunID:       "run-resolution-create-owner",
		StepID:      "step-resolution-create-owner",
		State:       common.DecisionTicketStateOpen,
		Reason:      "resolution create scope",
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub, BackgroundRoutine: magictask.NewBackgroundRoutine(1)}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandCreateResolution, decisionevents.CreateResolutionCommand{
		ResolutionID: "resolution-scope-create",
		TicketID:     ticketID,
		WorkspaceID:  "ws-resolution-create-other",
		RunID:        "run-resolution-create-other",
		StepID:       "step-resolution-create-other",
		Type:         "provide_missing_info",
		Payload:      mustDecisionEventPayload(t, map[string]any{"answer": "wrong scope"}),
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("create resolution with mismatched scope err = %v, want not found", err)
	}
	if _, err := resolutions.GetByTicketID(common.ContextWithRuntimeWorkspaceID(ctx, "ws-resolution-create-owner"), ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("owner resolution lookup err = %v, want not found after rejected create", err)
	}
}

func TestDecisionCreateResolutionCommandRejectsUnsupportedType(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-resolution-unsupported")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: "ws-resolution-unsupported",
		RunID:       "run-resolution-unsupported",
		StepID:      "step-resolution-unsupported",
		State:       common.DecisionTicketStateOpen,
		Reason:      "resolution type contract",
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandCreateResolution, decisionevents.CreateResolutionCommand{
		ResolutionID: "resolution-unsupported",
		TicketID:     ticketID,
		WorkspaceID:  "ws-resolution-unsupported",
		RunID:        "run-resolution-unsupported",
		StepID:       "step-resolution-unsupported",
		Type:         "unsupported_resolution",
		Payload:      mustDecisionEventPayload(t, map[string]any{"answer": "nope"}),
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create resolution unsupported type err = %v, want invalid argument", err)
	}
	if _, err := resolutions.GetByTicketID(common.ContextWithRuntimeWorkspaceID(ctx, "ws-resolution-unsupported"), ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("resolution lookup err = %v, want not found after rejected type", err)
	}
}

func TestDecisionResolutionCommandGetHonorsTicketScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
		Payload:  mustDecisionModelPayload(t, map[string]any{"answer": "ok"}),
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.GetResolutionCommand{
		WorkspaceID: "ws-resolution-owner-b",
		RunID:       "run-resolution-owner-a",
		TicketID:    ticketID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get resolution err = %v, want not found", err)
	}

	response, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.GetResolutionCommand{
		WorkspaceID: "ws-resolution-owner-a",
		RunID:       "run-resolution-owner-a",
		TicketID:    ticketID,
	})
	if err != nil {
		t.Fatalf("scoped get resolution err = %v, want response", err)
	}
	if response.Resolution == nil || response.Resolution.ID != "resolution-owner-a" || response.Resolution.TicketID != ticketID {
		t.Fatalf("response resolution = %+v, want scoped resolution", response.Resolution)
	}
}

func TestDecisionResolutionCommandWithoutScopeConflictsOnDuplicateOpenTickets(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.GetResolutionCommand{
		TicketID: ticketID,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate get resolution err = %v, want conflict", err)
	}

	_, err = sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandDeleteResolution, decisionevents.DeleteResolutionCommand{
		TicketID: ticketID,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate delete resolution err = %v, want conflict", err)
	}
}

func TestDecisionResolutionDeleteWithoutScopeDeletesUniqueTicketResolution(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
		Payload:  mustDecisionModelPayload(t, map[string]any{"answer": "ok"}),
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandDeleteResolution, decisionevents.DeleteResolutionCommand{
		TicketID: ticketID,
	})
	if err != nil {
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
		EventHub:    magicevent.NewHub(16),
	}
}

func TestDecisionUpdateTicketCommandPublishesUpdatedNotification(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
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
		Context:      mustDecisionModelPayload(t, map[string]any{"attempt": float64(1)}),
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	updated := make(chan decisionevents.TicketChangedData, 1)
	const observerID = "/decision/consumers/blocks/decision/module-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, rawHub)
	observer.Subscribe(decisionevents.TopicTicketUpdated, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(decisionevents.TicketChangedData); ok {
			updated <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(decisionevents.TopicTicketUpdated) })

	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandUpdateTicket, decisionevents.UpdateTicketCommand{
		TicketID:     ticketID,
		WorkspaceID:  "ws-ticket-update",
		RunID:        "run-ticket-update",
		StepID:       "step-ticket-update",
		Reason:       "provider unavailable",
		DecisionKind: "operator_input",
		Question:     "provide runtime config",
		Context:      mustDecisionEventPayload(t, map[string]any{"attempt": float64(2)}),
	})
	if err != nil {
		t.Fatalf("update ticket err = %v, want response", err)
	}

	item, err := tickets.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	attempt, _ := item.Context.Float64("attempt")
	if item.State != common.DecisionTicketStateResolved || attempt != float64(2) {
		t.Fatalf("ticket after update = %+v, want owner-preserved state with updated context", item)
	}

	select {
	case data := <-updated:
		if data.WorkspaceID != "ws-ticket-update" || data.RunID != "run-ticket-update" || data.StepID != "step-ticket-update" {
			t.Fatalf("updated scope = ws:%s run:%s step:%s", data.WorkspaceID, data.RunID, data.StepID)
		}
		if data.State != common.DecisionTicketStateResolved || data.Reason != "provider unavailable" {
			t.Fatalf("updated data = %+v, want owner-preserved resolved ticket", data)
		}
	case <-time.After(time.Second):
		t.Fatal("ticket update command did not publish ticket updated notification")
	}
}

func TestRuntimeGovernanceResolutionDoesNotResolveTicketBeforeAggregateCommit(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:                     "ticket-runtime-commit-order",
		WorkspaceID:            "ws-runtime-commit-order",
		RunID:                  "run-runtime-commit-order",
		StepID:                 "runtime-v2/dispatch-1",
		Reason:                 "confirm provider recovery",
		DecisionKind:           "runtime_v2_governance",
		State:                  common.DecisionTicketStateOpen,
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	module := New()
	if err := module.Configure(Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub, BackgroundRoutine: magictask.NewBackgroundRoutine(1)}); err != nil {
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

	_, err := sendDecisionTestCommand(ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.EnsureResolvedCommand{
		ResolutionID: "resolution-runtime-commit-order",
		TicketID:     "ticket-runtime-commit-order",
		WorkspaceID:  "ws-runtime-commit-order",
		RunID:        "run-runtime-commit-order",
		StepID:       "runtime-v2/dispatch-1",
		DecisionKind: "runtime_v2_governance",
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:      mustDecisionEventPayload(t, map[string]any{"summary": "provider is ready"}),
	})
	if err == nil {
		t.Fatal("resolve unexpectedly succeeded without an aggregate commit handler")
	}
	ticket, err := tickets.Get(ctx, "ticket-runtime-commit-order")
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen || ticket.RuntimeGovernanceProposal == nil || ticket.RuntimeGovernanceProposal.State != decisionmodel.RuntimeGovernanceResolutionProposalPending {
		t.Fatalf("ticket after failed aggregate commit = %+v", ticket)
	}
	if _, err := resolutions.GetByTicketID(ctx, "ticket-runtime-commit-order"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("formal resolution err = %v, want not found", err)
	}

	ackEvidence := subscribeDecisionEvidenceAck(rawHub)
	defer ackEvidence()
	commitObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	commitObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand)
		if !ok || command.ResolutionID != "resolution-runtime-commit-order" {
			result.Set(nil, nil)
			return
		}
		result.Set(orchestratorevents.CommitRuntimeGovernanceResolutionResult{
			ResolutionID: command.ResolutionID,
			Disposition:  orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate,
		}, nil)
	})
	defer commitObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket)

	module.bizPtr.ReconcileRuntimeGovernanceProposals(ctx)
	ticket, err = tickets.Get(ctx, "ticket-runtime-commit-order")
	if err != nil {
		t.Fatalf("read reconciled ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateResolved || ticket.RuntimeGovernanceProposal == nil || ticket.RuntimeGovernanceProposal.State != decisionmodel.RuntimeGovernanceResolutionProposalCommitted {
		t.Fatalf("ticket after reconciliation = %+v", ticket)
	}
	if _, err := resolutions.GetByTicketID(ctx, "ticket-runtime-commit-order"); err != nil {
		t.Fatalf("formal resolution after reconciliation: %v", err)
	}
}

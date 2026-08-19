package decision

import (
	"context"
	"errors"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustDecisionEventPayload(t *testing.T, input map[string]any) []workorcheventpayload.Field {
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

func sendDecisionTestCommand[Response any, Command any](ctx context.Context, rawHub magicevent.Hub, topic string, command Command) (Response, error) {
	return sendDecisionTestCommandFrom[Response](ctx, rawHub, topic, "decision/module_test", command)
}

func sendDecisionTestCommandFrom[Response any, Command any](ctx context.Context, rawHub magicevent.Hub, topic string, source string, command Command) (Response, error) {
	var zero Response
	ev := magicevent.NewEventWithContext(topic, source, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return zero, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return zero, err.Cause
		}
		return zero, err
	}
	data, err := result.Get()
	if err != nil {
		return zero, err
	}
	response, ok := data.(Response)
	if !ok {
		return zero, common.ErrInvalidArgument
	}
	return response, nil
}

func runtimeGovernanceResolutionCommand(ticketID common.TicketID, workspaceID common.WorkspaceID, runID common.RunID) decisionevents.EnsureResolvedCommand {
	return decisionevents.EnsureResolvedCommand{
		ResolutionID: "resolution-" + common.ResolutionID(ticketID),
		TicketID:     ticketID, WorkspaceID: workspaceID, RunID: runID, StepID: "runtime/dispatch-1",
		DecisionKind: "runtime_governance", Type: decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload: []workorcheventpayload.Field{{Name: "summary", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueString, String: "provider is ready"}}},
	}
}

func subscribeRuntimeGovernanceCommit(rawHub magicevent.Hub) func() {
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand)
		if !ok {
			result.Set(nil, cd.NewError(cd.IllegalParam, "wrong Runtime governance commit command"))
			return
		}
		result.Set(orchestratorevents.CommitRuntimeGovernanceResolutionResult{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID, ResolutionID: command.ResolutionID,
			Disposition:   orchestratorevents.RuntimeGovernanceResolutionDispositionCommitted,
			StateRevision: 1, AuthoritySnapshotID: "authority-runtime-governance-test", State: runtimecontract.ConvergenceStateNeedStateChange,
		}, nil)
	})
	return func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket) }
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
		deps decisionbiz.Dependencies
	}{
		{name: "missing tickets", deps: func() decisionbiz.Dependencies { d := base; d.Tickets = nil; return d }()},
		{name: "missing resolutions", deps: func() decisionbiz.Dependencies { d := base; d.Resolutions = nil; return d }()},
		{name: "missing event hub", deps: func() decisionbiz.Dependencies { d := base; d.EventHub = nil; return d }()},
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
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub, BackgroundRoutine: magictask.NewBackgroundRoutine(1)}); err != nil {
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

	_, err := sendDecisionTestCommand[decisionevents.GetTicketResult](ctx, rawHub, decisionevents.TopicDecisionCommandGetTicket, decisionevents.GetTicketCommand{
		WorkspaceID: "ws-ticket-owner-b",
		RunID:       "run-ticket-owner-a",
		StepID:      "step-ticket-owner-a",
		TicketID:    ticketID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	response, err := sendDecisionTestCommand[decisionevents.GetTicketResult](ctx, rawHub, decisionevents.TopicDecisionCommandGetTicket, decisionevents.GetTicketCommand{
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
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	response, err := sendDecisionTestCommand[decisionevents.ListTicketsByRunResult](ctx, rawHub, decisionevents.TopicDecisionCommandListTicketsByRun, decisionevents.ListTicketsByRunCommand{
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

func TestDecisionTicketListsAreExplicitBoundedAndCompositeCursorPaged(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	items := []*decisionmodel.Ticket{
		{ID: "ticket-z", WorkspaceID: "ws-page-a", RunID: "run-page-a", State: common.DecisionTicketStateOpen, Reason: "page", AllowedRecoveryActions: []string{"continue"}},
		{ID: "ticket-shared", WorkspaceID: "ws-page-a", RunID: "run-page-b", State: common.DecisionTicketStateOpen, Reason: "page"},
		{ID: "ticket-shared", WorkspaceID: "ws-page-b", RunID: "run-page-a", State: common.DecisionTicketStateOpen, Reason: "page"},
	}
	for _, item := range items {
		if err := tickets.Create(ctx, item); err != nil {
			t.Fatalf("create %s/%s: %v", item.WorkspaceID, item.ID, err)
		}
	}
	module := New()
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	if _, err := sendDecisionTestCommand[decisionevents.ListTicketsResult](ctx, rawHub, decisionevents.TopicDecisionCommandListTickets, decisionevents.ListTicketsCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("implicit global list err = %v, want invalid argument", err)
	}
	if _, err := sendDecisionTestCommand[decisionevents.ListTicketsResult](ctx, rawHub, decisionevents.TopicDecisionCommandListTickets, decisionevents.ListTicketsCommand{Scope: decisionevents.TicketListScopeAll, Limit: decisionevents.MaxListLimit + 1}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("oversized global list err = %v, want invalid argument", err)
	}
	first, err := sendDecisionTestCommand[decisionevents.ListTicketsResult](ctx, rawHub, decisionevents.TopicDecisionCommandListTickets, decisionevents.ListTicketsCommand{Scope: decisionevents.TicketListScopeAll, Limit: 2})
	if err != nil {
		t.Fatalf("first global page: %v", err)
	}
	if len(first.Tickets) != 2 || first.NextCursor == "" {
		t.Fatalf("first global page = %+v", first)
	}
	second, err := sendDecisionTestCommand[decisionevents.ListTicketsResult](ctx, rawHub, decisionevents.TopicDecisionCommandListTickets, decisionevents.ListTicketsCommand{Scope: decisionevents.TicketListScopeAll, Cursor: first.NextCursor, Limit: 2})
	if err != nil {
		t.Fatalf("second global page: %v", err)
	}
	if len(second.Tickets) != 1 || second.NextCursor != "" {
		t.Fatalf("second global page = %+v", second)
	}
	seen := map[string]struct{}{}
	for _, item := range append(first.Tickets, second.Tickets...) {
		seen[decisionevents.TicketCursor(item)] = struct{}{}
	}
	if len(seen) != 3 {
		t.Fatalf("global composite cursor lost or duplicated tickets: %+v", seen)
	}
	first.Tickets[0].AllowedRecoveryActions = []string{"mutated"}
	persisted, err := tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, first.Tickets[0].WorkspaceID), first.Tickets[0].ID)
	if err != nil {
		t.Fatalf("get persisted ticket: %v", err)
	}
	if len(persisted.AllowedRecoveryActions) > 0 && persisted.AllowedRecoveryActions[0] == "mutated" {
		t.Fatal("paged result shares mutable owner state")
	}

	workspacePage, err := sendDecisionTestCommand[decisionevents.ListTicketsByWorkspaceResult](ctx, rawHub, decisionevents.TopicDecisionCommandListTicketsByWS, decisionevents.ListTicketsByWorkspaceCommand{WorkspaceID: "ws-page-a", Limit: 1})
	if err != nil || len(workspacePage.Tickets) != 1 || workspacePage.NextCursor == "" {
		t.Fatalf("workspace page = %+v err=%v", workspacePage, err)
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
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand[decisionevents.EnsureResolvedResult](ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.EnsureResolvedCommand{
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

	purge, err := sendDecisionTestCommand[decisionevents.PurgeWorkspaceDecisionsResult](ctx, rawHub, decisionevents.TopicDecisionCommandPurgeWorkspace, decisionevents.PurgeWorkspaceDecisionsCommand{
		WorkspaceID: "ws-ticket-mutate-b",
	})
	if err != nil {
		t.Fatalf("purge unrelated workspace: %v", err)
	}
	if purge.Deleted != 0 {
		t.Fatalf("purge unrelated workspace deleted = %d, want 0", purge.Deleted)
	}
	if _, err := tickets.Get(ctx, ticketID); err != nil {
		t.Fatalf("ticket should remain after unrelated workspace purge: %v", err)
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
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	response, err := sendDecisionTestCommand[decisionevents.EnsureResolvedResult](ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.EnsureResolvedCommand{
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
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand[decisionevents.EnsureResolvedResult](ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.EnsureResolvedCommand{
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

func TestDecisionPurgeWorkspaceDeletesTicketAndResolutionAggregate(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-workspace-purge")
	workspaceID := common.WorkspaceID("ws-ticket-workspace-purge")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       "run-ticket-workspace-purge",
		State:       common.DecisionTicketStateResolved,
		Reason:      "decision workspace purge test",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := resolutions.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &decisionmodel.Resolution{
		ID:       "resolution-workspace-purge",
		TicketID: ticketID,
		Type:     "provide_missing_info",
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	module := New()
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	response, err := sendDecisionTestCommand[decisionevents.PurgeWorkspaceDecisionsResult](ctx, rawHub, decisionevents.TopicDecisionCommandPurgeWorkspace, decisionevents.PurgeWorkspaceDecisionsCommand{
		WorkspaceID: workspaceID,
	})
	if err != nil {
		t.Fatalf("purge workspace: %v", err)
	}
	if response.Deleted != 1 {
		t.Fatalf("purge deleted = %d, want 1", response.Deleted)
	}
	if _, err := tickets.Get(ctx, ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get purged ticket err = %v, want not found", err)
	}
	if _, err := resolutions.GetByTicketID(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), ticketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get purged resolution err = %v, want not found", err)
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
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand[decisionevents.GetResolutionResult](ctx, rawHub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.GetResolutionCommand{
		WorkspaceID: "ws-resolution-owner-b",
		RunID:       "run-resolution-owner-a",
		TicketID:    ticketID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get resolution err = %v, want not found", err)
	}

	response, err := sendDecisionTestCommand[decisionevents.GetResolutionResult](ctx, rawHub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.GetResolutionCommand{
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
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
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

	_, err := sendDecisionTestCommand[decisionevents.GetResolutionResult](ctx, rawHub, decisionevents.TopicDecisionCommandGetResolution, decisionevents.GetResolutionCommand{
		TicketID: ticketID,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate get resolution err = %v, want conflict", err)
	}

}

func decisionModuleContractDependencies() decisionbiz.Dependencies {
	return decisionbiz.Dependencies{
		Tickets:     memory.NewDecisionTicketRepository(),
		Resolutions: memory.NewDecisionResolutionRepository(),
		EventHub:    magicevent.NewHub(16),
	}
}

func TestRuntimeGovernanceOpenRequiresOrchestratorSourceAndMatchingWorkspaceContext(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	tickets := memory.NewDecisionTicketRepository()
	module := New()
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: memory.NewDecisionResolutionRepository(), EventHub: rawHub}); err != nil {
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
	command := decisionevents.OpenRuntimeGovernanceRequestCommand{
		TicketID: "ticket-runtime-ingress", WorkspaceID: "ws-runtime-ingress", RunID: "run-runtime-ingress", DispatchID: "dispatch-1",
		Transaction:            runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-runtime-ingress"},
		Dependency:             runtimecontract.ExternalDependency{Kind: "operator_input", SubjectRef: "provider", Reason: "confirm provider recovery"},
		ContinueResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		FailResolutionType:     decisioncommon.ResolutionTypeRejectCurrentPath,
	}
	if _, err := sendDecisionTestCommandFrom[decisionevents.OpenRuntimeGovernanceRequestResult](ctx, rawHub, decisionevents.TopicOpenRuntimeGovernanceRequest, "application/workorchd", command); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("wrong-source open err = %v, want invalid argument", err)
	}
	if items, err := tickets.List(ctx); err != nil || len(items) != 0 {
		t.Fatalf("wrong-source open persisted tickets = %+v err=%v", items, err)
	}

	mismatchedCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-other")
	if _, err := sendDecisionTestCommandFrom[decisionevents.OpenRuntimeGovernanceRequestResult](mismatchedCtx, rawHub, decisionevents.TopicOpenRuntimeGovernanceRequest, runtimecontract.RuntimeModuleOrchestrator, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong-context open err = %v, want conflict", err)
	}
	if items, err := tickets.List(ctx); err != nil || len(items) != 0 {
		t.Fatalf("wrong-context open persisted tickets = %+v err=%v", items, err)
	}

	response, err := sendDecisionTestCommandFrom[decisionevents.OpenRuntimeGovernanceRequestResult](common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), rawHub, decisionevents.TopicOpenRuntimeGovernanceRequest, runtimecontract.RuntimeModuleOrchestrator, command)
	if err != nil {
		t.Fatalf("valid Runtime governance open: %v", err)
	}
	if response.TicketID != command.TicketID || response.WorkspaceID != command.WorkspaceID || response.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID {
		t.Fatalf("open response = %+v", response)
	}
	item, err := tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), command.TicketID)
	if err != nil || item.WorkspaceID != command.WorkspaceID || item.CreatedNotificationPublishedAt == "" {
		t.Fatalf("persisted Runtime governance ticket = %+v err=%v", item, err)
	}
}

func TestRuntimeGovernanceInvalidCommitResultRemainsRecoverable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply func(magicevent.Result)
	}{
		{name: "nil", reply: func(result magicevent.Result) { result.Set(nil, nil) }},
		{name: "wrong type", reply: func(result magicevent.Result) { result.Set(struct{}{}, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			rawHub := magicevent.NewHub(16)
			tickets := memory.NewDecisionTicketRepository()
			resolutions := memory.NewDecisionResolutionRepository()
			ticketID := common.TicketID("ticket-invalid-commit-" + tc.name)
			workspaceID := common.WorkspaceID("ws-invalid-commit-" + tc.name)
			runID := common.RunID("run-invalid-commit-" + tc.name)
			if err := tickets.Create(ctx, &decisionmodel.Ticket{
				ID: ticketID, WorkspaceID: workspaceID, RunID: runID, StepID: "runtime/dispatch-1", Reason: "confirm recovery",
				DecisionKind: "runtime_governance", State: common.DecisionTicketStateOpen,
				AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
			}); err != nil {
				t.Fatalf("create ticket: %v", err)
			}
			module := New()
			if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
				t.Fatalf("Configure: %v", err)
			}
			if err := module.Start(); err != nil {
				t.Fatalf("Start: %v", err)
			}
			t.Cleanup(func() { _ = module.Stop(ctx) })
			observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
			observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, func(_ magicevent.Event, result magicevent.Result) { tc.reply(result) })
			t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket) })

			command := runtimeGovernanceResolutionCommand(ticketID, workspaceID, runID)
			if _, err := sendDecisionTestCommand[decisionevents.EnsureResolvedResult](ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, command); err == nil {
				t.Fatal("invalid commit result unexpectedly resolved ticket")
			}
			item, err := tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), ticketID)
			if err != nil {
				t.Fatalf("get pending ticket: %v", err)
			}
			if item.State != common.DecisionTicketStateOpen || item.RuntimeGovernanceProposal == nil || item.RuntimeGovernanceProposal.State != decisionmodel.RuntimeGovernanceResolutionProposalPending {
				t.Fatalf("ticket after invalid commit = %+v, want recoverable pending proposal", item)
			}
			if _, err := resolutions.GetByTicketID(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), ticketID); !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("resolution after invalid commit err = %v, want not found", err)
			}
			if err := module.bizPtr.ReconcileRuntimeGovernanceProposals(ctx); err == nil {
				t.Fatal("reconciliation silently discarded invalid commit result")
			}
			item, err = tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), ticketID)
			if err != nil || item.RuntimeGovernanceProposal.State != decisionmodel.RuntimeGovernanceResolutionProposalPending {
				t.Fatalf("ticket after failed reconciliation = %+v err=%v", item, err)
			}
		})
	}
}

func TestRuntimeGovernanceEvidenceFailureRecoversAfterFinalize(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	tickets := memory.NewDecisionTicketRepository()
	resolutions := memory.NewDecisionResolutionRepository()
	ticketID := common.TicketID("ticket-evidence-recovery")
	workspaceID := common.WorkspaceID("ws-evidence-recovery")
	runID := common.RunID("run-evidence-recovery")
	if err := tickets.Create(ctx, &decisionmodel.Ticket{
		ID: ticketID, WorkspaceID: workspaceID, RunID: runID, StepID: "runtime/dispatch-1", Reason: "confirm recovery",
		DecisionKind: "runtime_governance", State: common.DecisionTicketStateOpen,
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	module := New()
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	unsubCommit := subscribeRuntimeGovernanceCommit(rawHub)
	t.Cleanup(unsubCommit)
	evidenceFails := true
	evidenceCalls := 0
	evidenceObserver := magicevent.NewSimpleObserver(decisionevents.EvidenceCommandDestination, rawHub)
	evidenceObserver.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, func(event magicevent.Event, result magicevent.Result) {
		evidenceCalls++
		command, ok := event.Data().(decisionevents.RecordDecisionEvidenceCommand)
		if !ok {
			result.Set(nil, cd.NewError(cd.IllegalParam, "wrong evidence command"))
			return
		}
		if evidenceFails {
			result.Set(nil, cd.NewError(cd.Unexpected, "injected evidence failure"))
			return
		}
		result.Set(decisionevents.RecordDecisionEvidenceResult{EvidenceRecordID: command.EvidenceRecordID}, nil)
	})
	t.Cleanup(func() { evidenceObserver.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence) })

	command := runtimeGovernanceResolutionCommand(ticketID, workspaceID, runID)
	if _, err := sendDecisionTestCommand[decisionevents.EnsureResolvedResult](ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, command); err == nil {
		t.Fatal("resolution unexpectedly succeeded while evidence write failed")
	}
	item, err := tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), ticketID)
	if err != nil {
		t.Fatalf("get finalized ticket: %v", err)
	}
	if item.State != common.DecisionTicketStateResolved || item.RuntimeGovernanceProposal == nil || item.RuntimeGovernanceProposal.State != decisionmodel.RuntimeGovernanceResolutionProposalCommitted || item.RuntimeGovernanceProposal.EvidenceRecordedAt != "" {
		t.Fatalf("ticket after evidence failure = %+v", item)
	}
	if _, err := resolutions.GetByTicketID(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), ticketID); err != nil {
		t.Fatalf("formal resolution missing after finalize: %v", err)
	}

	evidenceFails = false
	if err := module.bizPtr.ReconcileRuntimeGovernanceProposals(ctx); err != nil {
		t.Fatalf("recover finalized evidence: %v", err)
	}
	item, err = tickets.Get(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), ticketID)
	if err != nil || item.RuntimeGovernanceProposal.EvidenceRecordedAt == "" || item.RuntimeGovernanceProposal.ResolutionNotificationPublishedAt == "" {
		t.Fatalf("recovered delivery markers = %+v err=%v", item, err)
	}
	callsAfterRecovery := evidenceCalls
	if err := module.bizPtr.ReconcileRuntimeGovernanceProposals(ctx); err != nil {
		t.Fatalf("idempotent reconciliation: %v", err)
	}
	if evidenceCalls != callsAfterRecovery {
		t.Fatalf("completed evidence was re-recorded: calls %d -> %d", callsAfterRecovery, evidenceCalls)
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
		StepID:                 "runtime/dispatch-1",
		Reason:                 "confirm provider recovery",
		DecisionKind:           "runtime_governance",
		State:                  common.DecisionTicketStateOpen,
		AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	module := New()
	if err := module.Configure(decisionbiz.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: rawHub, BackgroundRoutine: magictask.NewBackgroundRoutine(1)}); err != nil {
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

	_, err := sendDecisionTestCommand[decisionevents.EnsureResolvedResult](ctx, rawHub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.EnsureResolvedCommand{
		ResolutionID: "resolution-runtime-commit-order",
		TicketID:     "ticket-runtime-commit-order",
		WorkspaceID:  "ws-runtime-commit-order",
		RunID:        "run-runtime-commit-order",
		StepID:       "runtime/dispatch-1",
		DecisionKind: "runtime_governance",
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
			WorkspaceID: command.WorkspaceID, RunID: command.RunID,
			ResolutionID: command.ResolutionID, Disposition: orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate,
			StateRevision: 1, AuthoritySnapshotID: "authority-runtime-commit-order",
			State: runtimecontract.ConvergenceStateNeedStateChange,
		}, nil)
	})
	defer commitObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket)

	if err := module.bizPtr.ReconcileRuntimeGovernanceProposals(ctx); err != nil {
		t.Fatalf("reconcile Runtime governance proposals: %v", err)
	}
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

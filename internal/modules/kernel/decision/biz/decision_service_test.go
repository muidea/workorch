package decision

import (
	"context"
	"errors"
	"testing"

	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestService_CreateTicket_SeedsOpenState(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo, nil)

	cmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, cmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, cmd.TicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %q, want open", ticket.State)
	}
}

func TestService_CreateTicket_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewDecisionTicketRepository(), memory.NewDecisionResolutionRepository(), nil)

	tests := []decisionservice.CreateTicketCommand{
		{TicketID: "", RunID: common.RunID("run-1"), Reason: "need operator decision"},
		{TicketID: common.TicketID("ticket-1"), RunID: "", Reason: "need operator decision"},
		{TicketID: common.TicketID("ticket-1"), RunID: common.RunID("run-1"), Reason: ""},
	}

	for _, tt := range tests {
		if err := service.CreateTicket(ctx, tt); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("cmd %#v err = %v, want invalid argument", tt, err)
		}
	}
}

func TestService_CreateTicket_RejectsDuplicateTicketID(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo, nil)

	first := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-dup"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, first); err != nil {
		t.Fatalf("first create ticket: %v", err)
	}

	second := decisionservice.CreateTicketCommand{
		TicketID:    first.TicketID,
		WorkspaceID: common.WorkspaceID("ws-2"),
		RunID:       common.RunID("run-2"),
		StepID:      common.StepID("run-2/step-1"),
		Reason:      "other decision",
	}
	if err := service.CreateTicket(ctx, second); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second create err = %v, want conflict", err)
	}

	got, err := ticketRepo.Get(ctx, first.TicketID)
	if err != nil {
		t.Fatalf("get ticket after duplicate create: %v", err)
	}
	if got.RunID != first.RunID || got.Reason != first.Reason {
		t.Fatalf("ticket mutated after duplicate create: %+v", got)
	}
}

func TestService_EnsureTicket_IsIdempotentAndRejectsBindingMismatch(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	service := NewService(ticketRepo, memory.NewDecisionResolutionRepository(), nil)

	cmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-ensure"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.EnsureTicket(ctx, cmd); err != nil {
		t.Fatalf("first ensure ticket: %v", err)
	}
	if err := service.EnsureTicket(ctx, cmd); err != nil {
		t.Fatalf("second ensure ticket: %v", err)
	}

	if err := service.EnsureTicket(ctx, decisionservice.CreateTicketCommand{
		TicketID:    cmd.TicketID,
		WorkspaceID: common.WorkspaceID("ws-2"),
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      cmd.Reason,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("binding mismatch err = %v, want conflict", err)
	}

	if err := service.EnsureTicket(ctx, decisionservice.CreateTicketCommand{
		TicketID:    cmd.TicketID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      "other reason",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("reason mismatch err = %v, want conflict", err)
	}
}

func TestService_ResolveTicket_OpenToResolvedAndPersistsResolution(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo, nil)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	resolveCmd := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-1"),
		TicketID:     createCmd.TicketID,
		Type:         "approve-plan",
		Payload:      map[string]any{"actor": "operator"},
	}
	if err := service.ResolveTicket(ctx, resolveCmd); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get resolved ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateResolved {
		t.Fatalf("ticket state = %q, want resolved", ticket.State)
	}

	resolution, err := resolutionRepo.GetByTicketID(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	if resolution.ID != resolveCmd.ResolutionID {
		t.Fatalf("resolution id = %q, want %q", resolution.ID, resolveCmd.ResolutionID)
	}
	if resolution.Type != resolveCmd.Type {
		t.Fatalf("resolution type = %q, want %q", resolution.Type, resolveCmd.Type)
	}
}

func TestService_ResolveTicket_RejectsSecondResolveOnResolvedTicket(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo, nil)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	first := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-1"),
		TicketID:     createCmd.TicketID,
		Type:         "approve-plan",
	}
	if err := service.ResolveTicket(ctx, first); err != nil {
		t.Fatalf("first resolve ticket: %v", err)
	}

	second := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-2"),
		TicketID:     createCmd.TicketID,
		Type:         "change-plan",
	}
	if err := service.ResolveTicket(ctx, second); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second resolve err = %v, want conflict", err)
	}

	resolution, err := resolutionRepo.GetByTicketID(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get persisted resolution: %v", err)
	}
	if resolution.ID != first.ResolutionID {
		t.Fatalf("persisted resolution id = %q, want %q", resolution.ID, first.ResolutionID)
	}
}

func TestService_EnsureResolvedTicket_IsIdempotentAndRejectsResolutionMismatch(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo, nil)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-ensure-resolved"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	cmd := decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-ensure"),
		TicketID:     createCmd.TicketID,
		Type:         "approve-plan",
	}
	if err := service.EnsureResolvedTicket(ctx, cmd); err != nil {
		t.Fatalf("first ensure resolved ticket: %v", err)
	}
	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: cmd.ResolutionID,
		TicketID:     createCmd.TicketID,
		Type:         cmd.Type,
		Payload:      map[string]any{},
	}); err != nil {
		t.Fatalf("second ensure resolved ticket: %v", err)
	}
	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-ensure-other"),
		TicketID:     createCmd.TicketID,
		Type:         cmd.Type,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resolution id mismatch err = %v, want conflict", err)
	}
	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: cmd.ResolutionID,
		TicketID:     createCmd.TicketID,
		Type:         "change-plan",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resolution type mismatch err = %v, want conflict", err)
	}
	if err := service.EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: cmd.ResolutionID,
		TicketID:     createCmd.TicketID,
		Type:         cmd.Type,
		Payload:      map[string]any{"operator": "other"},
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("resolution payload mismatch err = %v, want conflict", err)
	}

	resolution, err := resolutionRepo.GetByTicketID(ctx, createCmd.TicketID)
	if err != nil {
		t.Fatalf("get persisted resolution: %v", err)
	}
	if resolution.ID != cmd.ResolutionID {
		t.Fatalf("persisted resolution id = %q, want %q", resolution.ID, cmd.ResolutionID)
	}
}

func TestService_ResolveTicket_ValidateArgumentsAndNotFound(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewDecisionTicketRepository(), memory.NewDecisionResolutionRepository(), nil)

	tests := []decisionservice.ResolveTicketCommand{
		{ResolutionID: "", TicketID: common.TicketID("ticket-1"), Type: "approve-plan"},
		{ResolutionID: common.ResolutionID("resolution-1"), TicketID: "", Type: "approve-plan"},
		{ResolutionID: common.ResolutionID("resolution-1"), TicketID: common.TicketID("ticket-1"), Type: ""},
	}

	for _, tt := range tests {
		if err := service.ResolveTicket(ctx, tt); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("cmd %#v err = %v, want invalid argument", tt, err)
		}
	}

	if err := service.ResolveTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-missing"),
		TicketID:     common.TicketID("missing"),
		Type:         "approve-plan",
	}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing ticket err = %v, want not found", err)
	}
}

func TestService_CreateAndResolveTicket_PublishesEvents(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	hub := events.NewMemoryHub()
	service := NewService(ticketRepo, resolutionRepo, hub)

	createCmd := decisionservice.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-event"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/manual-gate"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := service.ResolveTicket(ctx, decisionservice.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-event"),
		TicketID:     createCmd.TicketID,
		Type:         "approve-plan",
		Payload:      map[string]any{"operator": "workorch-demo"},
	}); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}

	published := hub.Published()
	if len(published) != 2 {
		t.Fatalf("len(published) = %d, want 2", len(published))
	}
	if published[0].Topic != events.TopicTicketCreated {
		t.Fatalf("first topic = %s, want %s", published[0].Topic, events.TopicTicketCreated)
	}
	if published[0].Version != events.WorkorchEventVersion || published[1].Version != events.WorkorchEventVersion {
		t.Fatalf("event versions = [%s %s], want both %s", published[0].Version, published[1].Version, events.WorkorchEventVersion)
	}
	if published[1].Topic != events.TopicTicketResolved {
		t.Fatalf("second topic = %s, want %s", published[1].Topic, events.TopicTicketResolved)
	}
	if published[1].Payload[events.PayloadKeyType] != "approve-plan" {
		t.Fatalf("resolved type payload = %v, want approve-plan", published[1].Payload[events.PayloadKeyType])
	}
	if published[1].Payload[events.PayloadKeyState] != string(common.DecisionTicketStateResolved) {
		t.Fatalf("resolved state payload = %v, want resolved", published[1].Payload[events.PayloadKeyState])
	}
}

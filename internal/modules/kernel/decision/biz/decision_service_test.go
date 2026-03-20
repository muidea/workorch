package decision

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestService_CreateTicket_SeedsOpenState(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	cmd := contracts.CreateTicketCommand{
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

func TestService_ResolveTicket_OpenToResolvedAndPersistsResolution(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := contracts.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	resolveCmd := contracts.ResolveTicketCommand{
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
	service := NewService(ticketRepo, resolutionRepo)

	createCmd := contracts.CreateTicketCommand{
		TicketID:    common.TicketID("ticket-1"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Reason:      "need operator decision",
	}
	if err := service.CreateTicket(ctx, createCmd); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	first := contracts.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-1"),
		TicketID:     createCmd.TicketID,
		Type:         "approve-plan",
	}
	if err := service.ResolveTicket(ctx, first); err != nil {
		t.Fatalf("first resolve ticket: %v", err)
	}

	second := contracts.ResolveTicketCommand{
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

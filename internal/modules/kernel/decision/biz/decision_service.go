package decision

import (
	"context"

	decisionModel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
}

func NewService(tickets repository.DecisionTicketRepository, resolutions repository.DecisionResolutionRepository) *Service {
	return &Service{tickets: tickets, resolutions: resolutions}
}

func (s *Service) CreateTicket(ctx context.Context, cmd decisionservice.CreateTicketCommand) error {
	if cmd.TicketID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item := &decisionModel.Ticket{
		ID:          cmd.TicketID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      cmd.Reason,
		State:       common.DecisionTicketStateOpen,
	}
	return s.tickets.Create(ctx, item)
}

func (s *Service) ResolveTicket(ctx context.Context, cmd decisionservice.ResolveTicketCommand) error {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return common.ErrInvalidArgument
	}

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return err
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return common.ErrConflict
	}

	resolution := &decisionModel.Resolution{
		ID:       cmd.ResolutionID,
		TicketID: cmd.TicketID,
		Type:     cmd.Type,
		Payload:  cmd.Payload,
	}
	if err := s.resolutions.Create(ctx, resolution); err != nil {
		return err
	}

	ticket.State = common.DecisionTicketStateResolved
	return s.tickets.Update(ctx, ticket)
}

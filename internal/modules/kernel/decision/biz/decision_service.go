package decision

import (
	"context"

	decisionModel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	tickets repository.DecisionTicketRepository
}

func NewService(tickets repository.DecisionTicketRepository) *Service {
	return &Service{tickets: tickets}
}

func (s *Service) CreateTicket(ctx context.Context, cmd contracts.CreateTicketCommand) error {
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

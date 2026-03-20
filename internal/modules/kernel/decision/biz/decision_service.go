package decision

import (
	"context"

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
	if cmd.TicketID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}
	_ = ctx
	return nil
}

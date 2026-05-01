package repository

import (
	"context"

	decision "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type DecisionTicketRepository interface {
	Create(context.Context, *decision.Ticket) error
	Get(context.Context, common.TicketID) (*decision.Ticket, error)
	List(context.Context) ([]*decision.Ticket, error)
	ListByRunID(context.Context, common.RunID) ([]*decision.Ticket, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*decision.Ticket, error)
	Update(context.Context, *decision.Ticket) error
	Delete(context.Context, common.TicketID) error
}

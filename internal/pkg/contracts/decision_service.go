package contracts

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateTicketCommand struct {
	TicketID    common.TicketID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Reason      string
}

type ResolveTicketCommand struct {
	ResolutionID common.ResolutionID
	TicketID     common.TicketID
	Type         string
	Payload      map[string]any
}

type DecisionService interface {
	CreateTicket(context.Context, CreateTicketCommand) error
	ResolveTicket(context.Context, ResolveTicketCommand) error
}

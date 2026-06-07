package decision

import (
	"context"

	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateTicketCommand = decisioncontracts.CreateTicketCommand
type ResolveTicketCommand = decisioncontracts.ResolveTicketCommand

type Service interface {
	CreateTicket(context.Context, CreateTicketCommand) error
	EnsureTicket(context.Context, CreateTicketCommand) error
	ResolveTicket(context.Context, ResolveTicketCommand) error
	EnsureResolvedTicket(context.Context, ResolveTicketCommand) error
	EnsureResolvedTicketAndReturn(context.Context, ResolveTicketCommand) (*decisionmodel.Ticket, *decisionmodel.Resolution, error)
}

type Module struct {
	createTicket                  func(context.Context, CreateTicketCommand) error
	ensureTicket                  func(context.Context, CreateTicketCommand) error
	resolveTicket                 func(context.Context, ResolveTicketCommand) error
	ensureResolvedTicket          func(context.Context, ResolveTicketCommand) error
	ensureResolvedTicketAndReturn func(context.Context, ResolveTicketCommand) (*decisionmodel.Ticket, *decisionmodel.Resolution, error)
}

type Dependencies struct {
	CreateTicket                  func(context.Context, CreateTicketCommand) error
	EnsureTicket                  func(context.Context, CreateTicketCommand) error
	ResolveTicket                 func(context.Context, ResolveTicketCommand) error
	EnsureResolvedTicket          func(context.Context, ResolveTicketCommand) error
	EnsureResolvedTicketAndReturn func(context.Context, ResolveTicketCommand) (*decisionmodel.Ticket, *decisionmodel.Resolution, error)
}

func New(deps Dependencies) *Module {
	return &Module{
		createTicket:                  deps.CreateTicket,
		ensureTicket:                  deps.EnsureTicket,
		resolveTicket:                 deps.ResolveTicket,
		ensureResolvedTicket:          deps.EnsureResolvedTicket,
		ensureResolvedTicketAndReturn: deps.EnsureResolvedTicketAndReturn,
	}
}

func (m *Module) CreateTicket(ctx context.Context, cmd CreateTicketCommand) error {
	if m == nil || m.createTicket == nil {
		return common.ErrInvalidArgument
	}
	return m.createTicket(ctx, cmd)
}

func (m *Module) EnsureTicket(ctx context.Context, cmd CreateTicketCommand) error {
	if m == nil || m.ensureTicket == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureTicket(ctx, cmd)
}

func (m *Module) ResolveTicket(ctx context.Context, cmd ResolveTicketCommand) error {
	if m == nil || m.resolveTicket == nil {
		return common.ErrInvalidArgument
	}
	return m.resolveTicket(ctx, cmd)
}

func (m *Module) EnsureResolvedTicket(ctx context.Context, cmd ResolveTicketCommand) error {
	if m == nil || m.ensureResolvedTicket == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureResolvedTicket(ctx, cmd)
}

func (m *Module) EnsureResolvedTicketAndReturn(ctx context.Context, cmd ResolveTicketCommand) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	if m == nil || m.ensureResolvedTicketAndReturn == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return m.ensureResolvedTicketAndReturn(ctx, cmd)
}

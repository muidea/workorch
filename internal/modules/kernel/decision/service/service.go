package decision

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateTicketCommand struct {
	TicketID     common.TicketID
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	StepID       common.StepID
	Reason       string
	DecisionKind string
	Question     string
	Context      map[string]any
}

type ResolveTicketCommand struct {
	ResolutionID common.ResolutionID
	TicketID     common.TicketID
	Type         string
	Payload      map[string]any
}

type Service interface {
	CreateTicket(context.Context, CreateTicketCommand) error
	EnsureTicket(context.Context, CreateTicketCommand) error
	ResolveTicket(context.Context, ResolveTicketCommand) error
	EnsureResolvedTicket(context.Context, ResolveTicketCommand) error
}

type Module struct {
	createTicket         func(context.Context, CreateTicketCommand) error
	ensureTicket         func(context.Context, CreateTicketCommand) error
	resolveTicket        func(context.Context, ResolveTicketCommand) error
	ensureResolvedTicket func(context.Context, ResolveTicketCommand) error
}

type Dependencies struct {
	CreateTicket         func(context.Context, CreateTicketCommand) error
	EnsureTicket         func(context.Context, CreateTicketCommand) error
	ResolveTicket        func(context.Context, ResolveTicketCommand) error
	EnsureResolvedTicket func(context.Context, ResolveTicketCommand) error
}

func New(deps Dependencies) *Module {
	return &Module{
		createTicket:         deps.CreateTicket,
		ensureTicket:         deps.EnsureTicket,
		resolveTicket:        deps.ResolveTicket,
		ensureResolvedTicket: deps.EnsureResolvedTicket,
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

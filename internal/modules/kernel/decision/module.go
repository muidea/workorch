package decision

import (
	"context"

	decisionbiz "github.com/muidea/workorch/internal/modules/kernel/decision/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = decisionservice.Service
type CreateTicketCommand = decisionservice.CreateTicketCommand
type ResolveTicketCommand = decisionservice.ResolveTicketCommand

type Dependencies struct {
	Tickets     repository.DecisionTicketRepository
	Resolutions repository.DecisionResolutionRepository
	EventHub    events.Hub
}

type Module struct {
	bizPtr     *decisionbiz.Service
	servicePtr *decisionservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Tickets == nil || deps.Resolutions == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = decisionbiz.NewService(deps.Tickets, deps.Resolutions, deps.EventHub)
	m.servicePtr = decisionservice.New(decisionservice.Dependencies{
		CreateTicket:                  m.bizPtr.CreateTicket,
		EnsureTicket:                  m.bizPtr.EnsureTicket,
		ResolveTicket:                 m.bizPtr.ResolveTicket,
		EnsureResolvedTicket:          m.bizPtr.EnsureResolvedTicket,
		EnsureResolvedTicketAndReturn: m.bizPtr.EnsureResolvedTicketAndReturn,
	})
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.servicePtr = nil
	m.bizPtr = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

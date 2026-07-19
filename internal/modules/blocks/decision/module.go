package decision

import (
	"context"

	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type CreateTicketCommand = decisioncontracts.CreateTicketCommand
type UpdateTicketCommand = decisioncontracts.UpdateTicketCommand
type ResolveTicketCommand = decisioncontracts.ResolveTicketCommand
type Dependencies = decisionbiz.Dependencies
type Module struct{ bizPtr *decisionbiz.Biz }

func New() *Module           { return &Module{} }
func (m *Module) ID() string { return ModuleID }
func (m *Module) Configure(d Dependencies) error {
	return m.configure(d)
}
func (m *Module) configure(d Dependencies) error {
	if m == nil || d.EventHub == nil || d.Tickets == nil || d.Resolutions == nil {
		return common.ErrInvalidArgument
	}
	x, err := decisionbiz.New(d.EventHub, d.BackgroundRoutine, d)
	if err != nil {
		return err
	}
	m.bizPtr = x
	return nil
}
func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.Start()
}
func (m *Module) Stop(ctx context.Context) error {
	if m != nil && m.bizPtr != nil {
		m.bizPtr.Teardown(ctx)
		m.bizPtr = nil
	}
	return nil
}

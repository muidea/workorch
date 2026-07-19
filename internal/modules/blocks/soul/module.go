package soul

import (
	"context"
	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	soulcontracts "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type CreateSoulCommand = soulcontracts.CreateSoulCommand
type UpdateSoulCommand = soulcontracts.UpdateSoulCommand
type ValidateCapabilitySelectionCommand = soulcontracts.ValidateCapabilitySelectionCommand
type Dependencies = soulbiz.Dependencies
type Module struct{ bizPtr *soulbiz.Biz }

func New() *Module                               { return &Module{} }
func (m *Module) ID() string                     { return ModuleID }
func (m *Module) Configure(d Dependencies) error { return m.configure(d) }
func (m *Module) configure(d Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if d.Souls == nil {
		return common.ErrInvalidArgument
	}
	if d.EventHub == nil {
		m.bizPtr = nil
		return nil
	}
	x, e := soulbiz.New(d.EventHub, d.BackgroundRoutine, d)
	if e != nil {
		return e
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
func (m *Module) Stop(c context.Context) error {
	if m != nil && m.bizPtr != nil {
		m.bizPtr.Teardown(c)
		m.bizPtr = nil
	}
	return nil
}

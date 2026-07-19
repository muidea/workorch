package mcp

import (
	"context"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpcontracts "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type CreateServerCommand = mcpcontracts.CreateServerCommand
type UpdateServerCommand = mcpcontracts.UpdateServerCommand
type Dependencies = mcpbiz.Dependencies
type Module struct{ bizPtr *mcpbiz.Biz }

func New() *Module                               { return &Module{} }
func (m *Module) ID() string                     { return ModuleID }
func (m *Module) Configure(d Dependencies) error { return m.configure(d) }
func (m *Module) configure(d Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if d.Servers == nil || d.Capabilities == nil {
		return common.ErrInvalidArgument
	}
	if d.EventHub == nil {
		return common.ErrInvalidArgument
	}
	x, e := mcpbiz.New(d.EventHub, d.BackgroundRoutine, d)
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

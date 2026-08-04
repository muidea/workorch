package web

import (
	"context"

	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindSearch = modulecommon.StepKindSearch
const StepKindFetch = modulecommon.StepKindFetch

type Module struct{ bizPtr *webbiz.Biz }

func New() *Module                                         { return &Module{} }
func (m *Module) ID() string                               { return ModuleID }
func (m *Module) Configure(deps webbiz.Dependencies) error { return m.configure(deps) }
func (m *Module) configure(deps webbiz.Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if deps.EventHub == nil {
		m.bizPtr = nil
		return nil
	}
	biz, err := webbiz.NewWithDependencies(deps)
	if err != nil {
		return err
	}
	m.bizPtr = biz
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

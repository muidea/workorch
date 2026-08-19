package skill

import (
	"context"

	skillbiz "github.com/muidea/workorch/internal/modules/blocks/skill/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Module struct{ bizPtr *skillbiz.Biz }

func New() *Module                                           { return &Module{} }
func (m *Module) ID() string                                 { return ModuleID }
func (m *Module) Configure(deps skillbiz.Dependencies) error { return m.configure(deps) }
func (m *Module) configure(deps skillbiz.Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if deps.Skills == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	b, err := skillbiz.New(deps.EventHub, deps.BackgroundRoutine, deps)
	if err != nil {
		return err
	}
	m.bizPtr = b
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

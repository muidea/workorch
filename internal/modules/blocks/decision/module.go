package decision

import (
	"context"

	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Module struct{ bizPtr *decisionbiz.Biz }

func New() *Module           { return &Module{} }
func (m *Module) ID() string { return ModuleID }
func (m *Module) Configure(d decisionbiz.Dependencies) error {
	return m.configure(d)
}
func (m *Module) configure(d decisionbiz.Dependencies) error {
	if m == nil || d.EventHub == nil {
		return common.ErrInvalidArgument
	}
	if err := d.OwnerRepositories.Validate(); err != nil {
		return err
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

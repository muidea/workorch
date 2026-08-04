package approval

import (
	"context"

	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Dependencies = approvalbiz.Dependencies

type Module struct{ bizPtr *approvalbiz.Biz }

func New() *Module                                  { return &Module{} }
func (m *Module) ID() string                        { return ModuleID }
func (m *Module) Configure(deps Dependencies) error { return m.configure(deps) }

func (m *Module) configure(deps Dependencies) error {
	if m == nil || deps.EventHub == nil || deps.Approvals == nil {
		return common.ErrInvalidArgument
	}
	bizPtr, err := approvalbiz.New(deps.EventHub, deps.BackgroundRoutine, deps)
	if err != nil {
		return err
	}
	m.bizPtr = bizPtr
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

package orchestrator

import (
	"context"

	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

// orchestratorbiz.Dependencies is owned by the Runtime Biz. The module only forwards this
// construction input during lifecycle setup.

// Module only bridges the framework lifecycle; Runtime authority remains
// entirely inside orchestratorbiz.Biz.
type Module struct{ bizPtr *orchestratorbiz.Biz }

func New() *Module           { return &Module{} }
func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps orchestratorbiz.Dependencies) error {
	if m == nil || deps.CommandHub == nil {
		return common.ErrInvalidArgument
	}
	owner := orchestratorbiz.New()
	if err := owner.Configure(deps); err != nil {
		return err
	}
	m.bizPtr = owner
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.Start()
}

func (m *Module) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if m.bizPtr != nil {
		if err := m.bizPtr.Stop(ctx); err != nil {
			return err
		}
	}
	m.bizPtr = nil
	return nil
}

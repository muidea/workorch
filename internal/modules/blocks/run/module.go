package run

import (
	"context"

	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

// runbiz.Dependencies is owned by the Run Biz. This alias preserves the module's
// construction surface without retaining framework infrastructure types in
// the lifecycle bridge.

// Module is the framework lifecycle bridge. The Run owner state, typed
// command subscriptions, and SM-4 inventory contract are all owned by Biz.
type Module struct {
	bizPtr *runbiz.Biz
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps runbiz.Dependencies) error {
	if m == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	if err := deps.OwnerRepositories.Validate(); err != nil {
		return err
	}
	owner := runbiz.New()
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

func (m *Module) prepareRuntimeRecoveryInventory() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.PrepareRuntimeRecoveryInventory()
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

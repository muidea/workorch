package decision

import (
	"context"
	cd "github.com/muidea/magicCommon/def"
	"sync"

	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

type Module struct {
	mu     sync.Mutex
	bizPtr *decisionbiz.Biz
}

func New() *Module                                              { return &Module{} }
func (m *Module) ID() string                                    { return ModuleID }
func (m *Module) Configure(deps decisionbiz.Dependencies) error { return m.configure(deps) }
func (m *Module) configure(deps decisionbiz.Dependencies) error {
	if m == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bizPtr != nil {
		return common.ErrConflict
	}
	owner, err := decisionbiz.New(deps.EventHub, deps.BackgroundRoutine, deps)
	if err != nil {
		return err
	}
	m.bizPtr = owner
	return nil
}
func (m *Module) owner() *decisionbiz.Biz {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bizPtr
}
func (m *Module) Start() error {
	owner := m.owner()
	if owner == nil {
		return common.ErrInvalidArgument
	}
	return owner.Start()
}
func (m *Module) BeginShutdown(_ context.Context) {
	if owner := m.owner(); owner != nil {
		owner.BeginShutdown()
	}
}
func (m *Module) Quiesce(ctx context.Context) *cd.Error {
	if owner := m.owner(); owner != nil {
		if err := owner.Quiesce(ctx); err != nil {
			return cd.WrapError(cd.Unexpected, err, "owner quiesce failed")
		}
	}
	return nil
}
func (m *Module) Stop(ctx context.Context) error {
	owner := m.owner()
	if owner == nil {
		return nil
	}
	if err := owner.Teardown(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	if m.bizPtr == owner {
		m.bizPtr = nil
	}
	m.mu.Unlock()
	return nil
}

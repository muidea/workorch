package governance

import (
	"context"
	cd "github.com/muidea/magicCommon/def"
	"sync"

	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID

// Module 只桥接生命周期；Hub 与 owner 状态仍由 Biz 持有。
type Module struct {
	mu     sync.Mutex
	bizPtr *governancebiz.Service
}

func New() *Module           { return &Module{} }
func (m *Module) ID() string { return ModuleID }
func (m *Module) Configure(deps governancebiz.EventDependencies) error {
	if m == nil || deps.AutoGovernanceTraces == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bizPtr != nil {
		return common.ErrConflict
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = workorchconfig.Current
	}
	m.bizPtr = governancebiz.NewEventServiceWithDependencies(deps)
	return nil
}
func (m *Module) owner() *governancebiz.Service {
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
	if err := owner.StopChecked(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	if m.bizPtr == owner {
		m.bizPtr = nil
	}
	m.mu.Unlock()
	return nil
}

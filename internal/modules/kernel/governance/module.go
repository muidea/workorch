package governance

import (
	"context"

	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID

// Module only bridges the framework lifecycle. Governance use cases,
// subscriptions, and typed EventHub I/O live in governancebiz.Service.
type Module struct {
	bizPtr *governancebiz.Service
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps governancebiz.EventDependencies) error {
	if m == nil || deps.AutoGovernanceTraces == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = workorchconfig.Current
	}
	m.bizPtr = governancebiz.NewEventServiceWithDependencies(deps)
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.Start()
}

func (m *Module) Stop(context.Context) error {
	if m == nil {
		return nil
	}
	if m.bizPtr != nil {
		m.bizPtr.Stop()
	}
	m.bizPtr = nil
	return nil
}

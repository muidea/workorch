package eventstream

import (
	"context"

	eventstreambiz "github.com/muidea/workorch/internal/modules/blocks/eventstream/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID

// Module is only the framework lifecycle bridge. Event subscriptions,
// command handling, and publication are owned by eventstreambiz.Biz.
type Module struct {
	bizPtr *eventstreambiz.Biz
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps eventstreambiz.Dependencies) error {
	if m == nil || deps.EventRecords == nil || deps.WorkspaceDeletions == nil || deps.WorkspacePurger == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = eventstreambiz.New(deps)
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

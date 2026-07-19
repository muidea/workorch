package git

import (
	"context"

	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindRead = modulecommon.StepKindRead
const StepKindWrite = modulecommon.StepKindWrite

// Dependencies is retained as a construction input for the explicit test
// runtime. The production plugin only passes framework-owned dependencies to
// the Git Biz.
type Dependencies = gitbiz.Dependencies

// Module is only the framework lifecycle bridge. Git ownership, observers,
// typed handlers, and EventHub calls belong to biz.Biz.
type Module struct {
	bizPtr *gitbiz.Biz
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps ...Dependencies) error {
	return m.configure(deps...)
}

func (m *Module) configure(deps ...Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if len(deps) == 0 || deps[0].EventHub == nil {
		m.bizPtr = nil
		return nil
	}
	configured, err := gitbiz.New(deps[0].EventHub, deps[0].BackgroundRoutine, deps...)
	if err != nil {
		return err
	}
	m.bizPtr = configured
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

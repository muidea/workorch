package file

import (
	"context"

	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindList = modulecommon.StepKindList
const StepKindRead = modulecommon.StepKindRead
const StepKindEdit = modulecommon.StepKindEdit
const StepKindWrite = modulecommon.StepKindWrite
const StepKindPatchApply = modulecommon.StepKindPatchApply
const StepKindPatchPreview = modulecommon.StepKindPatchPreview
const StepKindPatchRevert = modulecommon.StepKindPatchRevert

// Module is only the framework lifecycle bridge. The File owner use case and
// all EventHub interaction belong to biz.Biz.
type Module struct {
	bizPtr *filebiz.Biz
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps ...filebiz.Dependencies) error {
	var config filebiz.Dependencies
	if len(deps) > 0 {
		config = deps[0]
	}
	return m.configure(config)
}

func (m *Module) configure(deps filebiz.Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if m.bizPtr != nil {
		return common.ErrConflict
	}
	if deps.EventHub == nil {
		m.bizPtr = nil
		return common.ErrInvalidArgument
	}
	configured, err := filebiz.NewWithDependencies(deps)
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
		if err := m.bizPtr.Teardown(ctx); err != nil {
			return err
		}
		m.bizPtr = nil
	}
	return nil
}

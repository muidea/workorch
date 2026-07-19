package ssh

import (
	"context"

	sshbiz "github.com/muidea/workorch/internal/modules/blocks/ssh/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Dependencies = sshbiz.Dependencies
type Module struct{ bizPtr *sshbiz.Biz }

func New() *Module           { return &Module{} }
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
	biz, err := sshbiz.New(deps[0].EventHub, deps[0].BackgroundRoutine)
	if err != nil {
		return err
	}
	m.bizPtr = biz
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

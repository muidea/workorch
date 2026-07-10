package ssh

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	sshbiz "github.com/muidea/workorch/internal/modules/blocks/ssh/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Module struct {
	runtimeAdapter *sshbiz.RuntimeAdapter
	executor       *runtimeevent.Executor
	eventHub       magicevent.Hub
	background     magictask.BackgroundRoutine
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub   magicevent.Hub
	Background magictask.BackgroundRoutine
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.eventHub = deps[0].EventHub
		m.background = deps[0].Background
	}
	m.runtimeAdapter = sshbiz.NewRuntimeAdapter()
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil || m.background == nil {
		return common.ErrInvalidArgument
	}
	if m.executor != nil {
		return nil
	}
	m.executor = runtimeevent.NewExecutor(ModuleID, m.eventHub, m.background, m.runtimeAdapter, StepKindCommand)
	return m.executor.Run()
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.executor != nil {
		m.executor.Teardown()
	}
	m.executor = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	m.background = nil
	return nil
}

package file

import (
	"context"

	magictask "github.com/muidea/magicCommon/task"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const ModuleID = modulecommon.ModuleID
const StepKindList = modulecommon.StepKindList
const StepKindRead = modulecommon.StepKindRead
const StepKindEdit = modulecommon.StepKindEdit

type Module struct {
	runtimeAdapter *filebiz.RuntimeAdapter
	executor       *runtimeevent.Executor
	eventHub       events.Hub
	background     magictask.BackgroundRoutine
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub   events.Hub
	Background magictask.BackgroundRoutine
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.eventHub = deps[0].EventHub
		m.background = deps[0].Background
	}
	m.runtimeAdapter = filebiz.NewRuntimeAdapter()
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil || m.background == nil {
		return common.ErrInvalidArgument
	}
	if m.executor != nil {
		return nil
	}
	m.executor = runtimeevent.NewExecutor(ModuleID, m.eventHub, m.background, m.runtimeAdapter, StepKindList, StepKindRead, StepKindEdit)
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

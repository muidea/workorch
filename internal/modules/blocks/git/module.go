package git

import (
	"context"

	magictask "github.com/muidea/magicCommon/task"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindRead = modulecommon.StepKindRead
const StepKindWrite = modulecommon.StepKindWrite

type Module struct {
	runtimeAdapter *gitbiz.RuntimeAdapter
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
	cfg, err := workorchconfig.Current()
	if err != nil {
		return err
	}
	adapter, err := gitbiz.NewRuntimeAdapterFromTimeoutString(
		true,
		cfg.Runtime.Git.DefaultTimeout,
	)
	if err != nil {
		return err
	}
	m.runtimeAdapter = adapter
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil {
		return common.ErrInvalidArgument
	}
	if m.eventHub == nil || m.background == nil {
		return common.ErrInvalidArgument
	}
	if m.executor != nil {
		return nil
	}
	m.executor = runtimeevent.NewExecutor(ModuleID, m.eventHub, m.background, m.runtimeAdapter, StepKindRead, StepKindWrite)
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

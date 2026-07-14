package governance

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
	EventHub             magicevent.Hub
	LoadConfig           func() (workorchconfig.Config, error)
	BackgroundRoutine    magictask.BackgroundRoutine
}

type Module struct {
	bizPtr   *governancebiz.Service
	traces   repository.AutoGovernanceTraceRepository
	eventHub magicevent.Hub
	observer magicevent.SimpleObserver
	topics   []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.AutoGovernanceTraces == nil ||
		deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = workorchconfig.Current
	}
	m.bizPtr = governancebiz.NewEventService(
		deps.AutoGovernanceTraces,
		deps.EventHub,
		deps.LoadConfig,
		deps.BackgroundRoutine,
	)
	m.traces = deps.AutoGovernanceTraces
	m.eventHub = deps.EventHub
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	if err := m.bizPtr.Start(); err != nil {
		return err
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{governanceevents.TopicGovernanceAutoCommandListTraces}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m != nil && m.bizPtr != nil {
		m.bizPtr.Stop()
		m.bizPtr = nil
	}
	if m != nil {
		if m.observer != nil {
			for idx := len(m.topics) - 1; idx >= 0; idx-- {
				m.observer.Unsubscribe(m.topics[idx])
			}
		}
		m.topics = nil
		m.observer = nil
		m.traces = nil
		m.eventHub = nil
	}
	return nil
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setGovernanceMagicResult(result, response, err)
}

func (m *Module) executeCommand(ev magicevent.Event) (governanceevents.GovernanceCommandResult, error) {
	if m == nil || m.traces == nil {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	switch ev.ID() {
	case governanceevents.TopicGovernanceAutoCommandListTraces:
		cmd, ok := ev.Data().(governanceevents.ListAutoGovernanceTracesCommand)
		if !ok {
			return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.traces.List(ev.Context(), cmd.Query)
		if err != nil {
			return governanceevents.GovernanceCommandResult{}, err
		}
		return governanceevents.GovernanceCommandResult{Traces: items}, nil
	default:
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
}

func setGovernanceMagicResult(result magicevent.Result, data governanceevents.GovernanceCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

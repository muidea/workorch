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
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID

type ResolveDecisionFunc = governancebiz.ResolveDecisionFunc
type ResolveApprovalFunc = governancebiz.ResolveApprovalFunc
type ResumeRunFunc = governancebiz.ResumeRunFunc

type Dependencies struct {
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
	EventHub             magicevent.Hub
	TicketReader         governancebiz.TicketReader
	ResolutionReader     governancebiz.ResolutionReader
	ApprovalReader       governancebiz.ApprovalReader
	RunReader            governancebiz.RunReader
	WorkspaceDefinitions governancebiz.WorkspaceDefinitionReader
	MemoryReader         governancebiz.MemoryReader
	ResolveDecision      ResolveDecisionFunc
	ResolveApproval      ResolveApprovalFunc
	ResumeRun            ResumeRunFunc
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
	eventHubBridge := events.NewMagicHubWithHubOwner(deps.EventHub, ModuleID)
	if deps.ResolveDecision == nil {
		deps.ResolveDecision = eventResolveDecision(eventHubBridge)
	}
	if deps.ResolveApproval == nil {
		deps.ResolveApproval = eventResolveApproval(eventHubBridge)
	}
	if deps.ResumeRun == nil {
		deps.ResumeRun = eventResumeRun(eventHubBridge)
	}
	if deps.TicketReader == nil {
		deps.TicketReader = eventTicketReader{hub: eventHubBridge}
	}
	if deps.ResolutionReader == nil {
		deps.ResolutionReader = eventResolutionReader{hub: eventHubBridge}
	}
	if deps.ApprovalReader == nil {
		deps.ApprovalReader = eventApprovalReader{hub: eventHubBridge}
	}
	if deps.RunReader == nil {
		deps.RunReader = eventRunReader{hub: eventHubBridge}
	}
	if deps.WorkspaceDefinitions == nil {
		deps.WorkspaceDefinitions = eventWorkspaceReader{hub: eventHubBridge}
	}
	if deps.MemoryReader == nil {
		deps.MemoryReader = eventMemoryReader{hub: eventHubBridge}
	}
	m.bizPtr = governancebiz.NewService(governancebiz.Dependencies{
		Tickets:              deps.TicketReader,
		Resolutions:          deps.ResolutionReader,
		Approvals:            deps.ApprovalReader,
		Runs:                 deps.RunReader,
		WorkspaceDefinitions: deps.WorkspaceDefinitions,
		MemoryItems:          deps.MemoryReader,
		AutoGovernanceTraces: deps.AutoGovernanceTraces,
		EventHub:             eventHubBridge,
		ResolveDecision:      deps.ResolveDecision,
		ResolveApproval:      deps.ResolveApproval,
		ResumeRun:            deps.ResumeRun,
		LoadConfig:           deps.LoadConfig,
		BackgroundRoutine:    deps.BackgroundRoutine,
	})
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
	response, err := m.executeCommand(ev.Context(), ev.ID(), ev.Data())
	setGovernanceMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (governanceevents.GovernanceCommandResult, error) {
	if m == nil || m.traces == nil {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case governanceevents.TopicGovernanceAutoCommandListTraces:
		cmd, ok := data.(governanceevents.ListAutoGovernanceTracesCommand)
		if !ok {
			return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.traces.List(ctx, cmd.Query)
		if err != nil {
			return governanceevents.GovernanceCommandResult{}, err
		}
		return governanceevents.GovernanceCommandResult{Traces: items}, nil
	default:
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
}

func setGovernanceMagicResult(result magicevent.Result, data any, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

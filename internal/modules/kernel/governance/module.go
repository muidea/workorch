package governance

import (
	"context"

	magictask "github.com/muidea/magicCommon/task"
	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
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
	EventHub             events.Hub
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
	eventHub events.Hub
	unsub    []func()
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
	if deps.ResolveDecision == nil {
		deps.ResolveDecision = eventResolveDecision(deps.EventHub)
	}
	if deps.ResolveApproval == nil {
		deps.ResolveApproval = eventResolveApproval(deps.EventHub)
	}
	if deps.ResumeRun == nil {
		deps.ResumeRun = eventResumeRun(deps.EventHub)
	}
	if deps.TicketReader == nil {
		deps.TicketReader = eventTicketReader{hub: deps.EventHub}
	}
	if deps.ResolutionReader == nil {
		deps.ResolutionReader = eventResolutionReader{hub: deps.EventHub}
	}
	if deps.ApprovalReader == nil {
		deps.ApprovalReader = eventApprovalReader{hub: deps.EventHub}
	}
	if deps.RunReader == nil {
		deps.RunReader = eventRunReader{hub: deps.EventHub}
	}
	if deps.WorkspaceDefinitions == nil {
		deps.WorkspaceDefinitions = eventWorkspaceReader{hub: deps.EventHub}
	}
	if deps.MemoryReader == nil {
		deps.MemoryReader = eventMemoryReader{hub: deps.EventHub}
	}
	m.bizPtr = governancebiz.NewService(governancebiz.Dependencies{
		Tickets:              deps.TicketReader,
		Resolutions:          deps.ResolutionReader,
		Approvals:            deps.ApprovalReader,
		Runs:                 deps.RunReader,
		WorkspaceDefinitions: deps.WorkspaceDefinitions,
		MemoryItems:          deps.MemoryReader,
		AutoGovernanceTraces: deps.AutoGovernanceTraces,
		EventHub:             deps.EventHub,
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
	if len(m.unsub) > 0 {
		return nil
	}
	if err := m.bizPtr.Start(); err != nil {
		return err
	}
	m.unsub = append(m.unsub, m.eventHub.Subscribe(events.TopicGovernanceAutoCommandListTraces, m.handleListTracesCommand))
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m != nil && m.bizPtr != nil {
		m.bizPtr.Stop()
		m.bizPtr = nil
	}
	if m != nil {
		for idx := len(m.unsub) - 1; idx >= 0; idx-- {
			if m.unsub[idx] != nil {
				m.unsub[idx]()
			}
		}
		m.unsub = nil
		m.traces = nil
		m.eventHub = nil
	}
	return nil
}

func (m *Module) handleListTracesCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.traces == nil {
		return common.ErrInvalidArgument
	}
	query := autoGovernanceTraceQueryFromPayload(envelope.Payload)
	items, err := m.traces.List(ctx, query)
	if err != nil {
		return err
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload:      map[string]any{events.PayloadKeyItems: items},
	})
}

func autoGovernanceTraceQueryFromPayload(payload map[string]any) repository.AutoGovernanceTraceQuery {
	if payload == nil {
		return repository.AutoGovernanceTraceQuery{}
	}
	if query, ok := payload[events.PayloadKeyQuery].(repository.AutoGovernanceTraceQuery); ok {
		return query
	}
	return repository.AutoGovernanceTraceQuery{
		WorkspaceID: common.WorkspaceID(stringFromPayload(payload, events.PayloadKeyWorkspaceID)),
		RunID:       common.RunID(stringFromPayload(payload, events.PayloadKeyRunID)),
		StepID:      common.StepID(stringFromPayload(payload, events.PayloadKeyStepID)),
		ItemType:    stringFromPayload(payload, events.PayloadKeyItemType),
		ItemID:      stringFromPayload(payload, events.PayloadKeyItemID),
		Status:      stringFromPayload(payload, events.PayloadKeyStatus),
		Limit:       intFromPayload(payload, events.PayloadKeyLimit),
	}
}

func intFromPayload(payload map[string]any, key string) int {
	switch value := payload[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

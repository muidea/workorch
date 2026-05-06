package governance

import (
	"context"

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
	Tickets              repository.DecisionTicketRepository
	Resolutions          repository.DecisionResolutionRepository
	Approvals            repository.ApprovalRepository
	Runs                 repository.RunRepository
	Goals                repository.GoalRepository
	DoDs                 repository.DoDRepository
	MemoryItems          repository.MemoryItemRepository
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
	EventHub             events.Hub
	ResolveDecision      ResolveDecisionFunc
	ResolveApproval      ResolveApprovalFunc
	ResumeRun            ResumeRunFunc
	LoadConfig           func() (workorchconfig.Config, error)
}

type Module struct {
	bizPtr *governancebiz.Service
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Tickets == nil ||
		deps.Resolutions == nil ||
		deps.Approvals == nil ||
		deps.Runs == nil ||
		deps.Goals == nil ||
		deps.DoDs == nil ||
		deps.MemoryItems == nil ||
		deps.AutoGovernanceTraces == nil ||
		deps.EventHub == nil ||
		deps.ResolveDecision == nil ||
		deps.ResolveApproval == nil ||
		deps.ResumeRun == nil {
		return common.ErrInvalidArgument
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = workorchconfig.Current
	}
	m.bizPtr = governancebiz.NewService(governancebiz.Dependencies{
		Tickets:              deps.Tickets,
		Resolutions:          deps.Resolutions,
		Approvals:            deps.Approvals,
		Runs:                 deps.Runs,
		Goals:                deps.Goals,
		DoDs:                 deps.DoDs,
		MemoryItems:          deps.MemoryItems,
		AutoGovernanceTraces: deps.AutoGovernanceTraces,
		EventHub:             deps.EventHub,
		ResolveDecision:      deps.ResolveDecision,
		ResolveApproval:      deps.ResolveApproval,
		ResumeRun:            deps.ResumeRun,
		LoadConfig:           deps.LoadConfig,
	})
	return nil
}

func (m *Module) Run() error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.Start()
}

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	if m != nil && m.bizPtr != nil {
		m.bizPtr.Stop()
		m.bizPtr = nil
	}
	return nil
}

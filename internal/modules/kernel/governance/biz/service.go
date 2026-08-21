package governance

import (
	"context"
	"sync"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type ResolveDecisionFunc func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error
type ResolveApprovalFunc func(context.Context, common.ApprovalID, common.RunID) error

type TicketReader interface {
	Get(context.Context, common.TicketID) (*decisionmodel.Ticket, error)
}

type ApprovalReader interface {
	GetScoped(context.Context, common.WorkspaceID, common.RunID, common.ApprovalID) (*approvalmodel.Approval, error)
}

type RunReader interface {
	Get(context.Context, common.RunID) (*runmodel.Run, error)
	GetByWorkspaceID(context.Context, common.WorkspaceID, common.RunID) (*runmodel.Run, error)
	List(context.Context) ([]*runmodel.Run, error)
}

type WorkspaceDefinitionReader interface {
	GetGoal(context.Context, common.GoalID) (*workspace.Goal, error)
	GetDoD(context.Context, common.DoDID) (*workspace.DoD, error)
}

type MemoryReader interface {
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
}

type servicePorts struct {
	Tickets              TicketReader
	Approvals            ApprovalReader
	Runs                 RunReader
	WorkspaceDefinitions WorkspaceDefinitionReader
	MemoryItems          MemoryReader
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
	// NotificationHub is test-fixture assembly input. Production uses
	// NewEventService and stores the Hub only in Base.
	NotificationHub   magicevent.Hub
	ResolveDecision   ResolveDecisionFunc
	ResolveApproval   ResolveApprovalFunc
	LoadConfig        func() (workorchconfig.Config, error)
	BackgroundRoutine magictask.BackgroundRoutine
}

// EventDependencies are the production Governance owner inputs. Test-only
// port assembly remains private to this package.
type EventDependencies struct {
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
	EventHub             magicevent.Hub
	LoadConfig           func() (workorchconfig.Config, error)
	BackgroundRoutine    magictask.BackgroundRoutine
}

type Service struct {
	basebiz.Base
	tickets         TicketReader
	approvals       ApprovalReader
	runs            RunReader
	workspaceDefs   WorkspaceDefinitionReader
	memoryItems     MemoryReader
	traces          repository.AutoGovernanceTraceRepository
	resolve         ResolveDecisionFunc
	resolveApproval ResolveApprovalFunc
	loadConfig      func() (workorchconfig.Config, error)
	background      magictask.BackgroundRoutine
	mu              sync.Mutex
	unsubscribes    []func()
	topics          []string
	workCtx         context.Context
	workCancel      context.CancelFunc
	workQueue       chan governanceWork
	workDone        chan struct{}
	reconcileCancel context.CancelFunc
	reconcileDone   chan struct{}
}

type governanceWork func(context.Context)

func NewEventService(traces repository.AutoGovernanceTraceRepository, commandHub magicevent.Hub, loadConfig func() (workorchconfig.Config, error), background magictask.BackgroundRoutine) *Service {
	service := newServiceWithPorts(servicePorts{
		AutoGovernanceTraces: traces,
		NotificationHub:      commandHub,
		LoadConfig:           loadConfig,
		BackgroundRoutine:    background,
	})
	send := service.SendEvent
	service.tickets = eventTicketReader{send: send}
	service.approvals = eventApprovalReader{send: send}
	service.runs = eventRunReader{send: send}
	service.workspaceDefs = eventWorkspaceReader{send: send}
	service.memoryItems = eventMemoryReader{send: send}
	service.resolve = eventResolveDecision(send)
	service.resolveApproval = eventResolveApproval(send)
	return service
}

func NewEventServiceWithDependencies(deps EventDependencies) *Service {
	return NewEventService(deps.AutoGovernanceTraces, deps.EventHub, deps.LoadConfig, deps.BackgroundRoutine)
}

func newServiceWithPorts(deps servicePorts) *Service {
	return &Service{
		Base:            basebiz.New(governancecommon.ModuleID, deps.NotificationHub, deps.BackgroundRoutine),
		tickets:         deps.Tickets,
		approvals:       deps.Approvals,
		runs:            deps.Runs,
		workspaceDefs:   deps.WorkspaceDefinitions,
		memoryItems:     deps.MemoryItems,
		traces:          deps.AutoGovernanceTraces,
		resolve:         deps.ResolveDecision,
		resolveApproval: deps.ResolveApproval,
		loadConfig:      deps.LoadConfig,
		background:      deps.BackgroundRoutine,
	}
}

func (s *Service) Start() error {
	if s == nil || s.EventHub() == nil || s.tickets == nil || s.approvals == nil || s.runs == nil || s.workspaceDefs == nil || s.memoryItems == nil || s.traces == nil || s.resolve == nil || s.resolveApproval == nil {
		return common.ErrInvalidArgument
	}
	if s.loadConfig == nil {
		s.loadConfig = workorchconfig.Current
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.unsubscribes) > 0 {
		return nil
	}
	workCtx, workCancel := context.WithCancel(context.Background())
	workQueue := make(chan governanceWork, 256)
	workDone := make(chan struct{})
	s.workCtx = workCtx
	s.workCancel = workCancel
	s.workQueue = workQueue
	s.workDone = workDone
	if err := s.startGovernanceWorker(workCtx, workQueue, workDone); err != nil {
		s.workCtx = nil
		s.workCancel = nil
		s.workQueue = nil
		s.workDone = nil
		workCancel()
		return err
	}
	var unsubTicket func()
	if s.EventHub() != nil {
		const observerID = "/decision/consumers/kernel/governance"
		s.SubscribeMatchedFunc(observerID, observerID, decisionevents.TopicTicketCreated, func(event magicevent.Event, _ magicevent.Result) {
			if data, ok := event.Data().(decisionevents.TicketChangedData); ok {
				if data.DecisionKind == "workspace_definition_clarification" || data.DecisionKind == "runtime_definition_clarification" {
					return
				}
				s.enqueueGovernanceWork(func(workCtx context.Context) {
					s.processTicketCreated(workCtx, data)
				})
			}
		})
		unsubTicket = func() { s.UnsubscribeMatchedFunc(observerID, decisionevents.TopicTicketCreated) }
	}
	var unsubRunState func()
	if s.EventHub() != nil {
		const observerID = "/run/consumers/kernel/governance"
		s.SubscribeMatchedFunc(observerID, observerID, runevents.TopicRunStateChanged, func(event magicevent.Event, _ magicevent.Result) {
			if data, ok := event.Data().(runevents.RunStateChangedData); ok {
				s.enqueueGovernanceWork(func(workCtx context.Context) {
					s.processRunStateChanged(workCtx, data)
				})
			}
		})
		unsubRunState = func() { s.UnsubscribeMatchedFunc(observerID, runevents.TopicRunStateChanged) }
	}
	s.unsubscribes = []func(){unsubTicket, unsubRunState}
	s.topics = []string{governanceevents.TopicGovernanceAutoCommandListTraces}
	for _, topic := range s.topics {
		s.SubscribeFunc(topic, s.handleCommand)
	}
	if err := s.scheduleBlockedRunReconciliationLocked(); err != nil {
		for _, unsub := range s.unsubscribes {
			if unsub != nil {
				unsub()
			}
		}
		s.unsubscribes = nil
		s.workCtx = nil
		s.workCancel = nil
		s.workQueue = nil
		s.workDone = nil
		workCancel()
		return err
	}
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	for idx := len(s.topics) - 1; idx >= 0; idx-- {
		s.UnsubscribeFunc(s.topics[idx])
	}
	s.topics = nil
	for _, unsub := range s.unsubscribes {
		if unsub != nil {
			unsub()
		}
	}
	s.unsubscribes = nil
	cancel := s.reconcileCancel
	done := s.reconcileDone
	workCancel := s.workCancel
	workDone := s.workDone
	s.reconcileCancel = nil
	s.reconcileDone = nil
	s.workCtx = nil
	s.workCancel = nil
	s.workQueue = nil
	s.workDone = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	if workCancel != nil {
		workCancel()
	}
	if workDone != nil {
		select {
		case <-workDone:
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *Service) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := s.executeCommand(ev)
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response.Clone(), nil)
}

func (s *Service) executeCommand(ev magicevent.Event) (governanceevents.GovernanceCommandResult, error) {
	if s == nil || s.traces == nil {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	if ev.ID() != governanceevents.TopicGovernanceAutoCommandListTraces {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(governanceevents.ListAutoGovernanceTracesCommand)
	if !ok {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	items, err := s.traces.List(ev.Context(), cmd.Query)
	if err != nil {
		return governanceevents.GovernanceCommandResult{}, err
	}
	return governanceevents.GovernanceCommandResult{Traces: items}, nil
}

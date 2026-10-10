package governance

import (
	"context"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runcommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
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
	mu              sync.Mutex
	traceMu         sync.Mutex
	lifecycle       basebiz.Lifecycle
	startAttempted  bool
	started         bool
	closing         bool
	releasing       bool
	released        bool
	unsubscribes    []func() error
	topics          []string
	workCtx         context.Context
	workCancel      context.CancelFunc
	workQueue       chan governanceWork
	workEnqueue     sync.WaitGroup
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
	}
}

func (s *Service) Start() error {
	if s == nil || s.EventHub() == nil || s.tickets == nil || s.approvals == nil || s.runs == nil || s.workspaceDefs == nil || s.memoryItems == nil || s.traces == nil || s.resolve == nil || s.resolveApproval == nil {
		return common.ErrInvalidArgument
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return basebiz.ErrLifecycleClosing
	}
	if s.startAttempted {
		started := s.started
		s.mu.Unlock()
		if started {
			return nil
		}
		return common.ErrConflict
	}
	if err := s.lifecycle.Reset(); err != nil {
		s.mu.Unlock()
		return err
	}
	leave, err := s.lifecycle.Enter()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.startAttempted = true
	if s.loadConfig == nil {
		s.loadConfig = workorchconfig.Current
	}
	workCtx, workCancel := context.WithCancel(context.Background())
	workQueue, workDone := make(chan governanceWork, 256), make(chan struct{})
	s.workCtx, s.workCancel, s.workQueue, s.workDone = workCtx, workCancel, workQueue, workDone
	s.mu.Unlock()
	defer leave()
	if err := s.startGovernanceWorker(workCtx, workQueue, workDone); err != nil {
		return err
	}
	const ticketObserverID = "/decision/consumers/kernel/governance"
	if err := s.SubscribeMatchedFuncChecked(ticketObserverID, ticketObserverID, decisionevents.TopicTicketCreated, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := governanceTicketCreatedData(event); ok {
			if data.DecisionKind == "workspace_definition_clarification" || data.DecisionKind == "runtime_definition_clarification" {
				return
			}
			s.enqueueGovernanceWork(func(workCtx context.Context) { s.processTicketCreated(workCtx, data) })
		}
	}); err != nil {
		return err
	}
	s.mu.Lock()
	s.unsubscribes = append(s.unsubscribes, func() error {
		return s.UnsubscribeMatchedFuncChecked(ticketObserverID, decisionevents.TopicTicketCreated)
	})
	s.mu.Unlock()
	const runObserverID = "/run/consumers/kernel/governance"
	if err := s.SubscribeMatchedFuncChecked(runObserverID, runObserverID, runevents.TopicRunStateChanged, func(event magicevent.Event, _ magicevent.Result) {
		if data, ok := governanceRunStateChangedData(event); ok {
			s.enqueueGovernanceWork(func(workCtx context.Context) { s.processRunStateChanged(workCtx, data) })
		}
	}); err != nil {
		return err
	}
	s.mu.Lock()
	s.unsubscribes = append(s.unsubscribes, func() error { return s.UnsubscribeMatchedFuncChecked(runObserverID, runevents.TopicRunStateChanged) })
	s.mu.Unlock()
	const topic = governanceevents.TopicGovernanceAutoCommandListTraces
	if err := s.SubscribeFuncChecked(topic, func(ev magicevent.Event, result magicevent.Result) {
		leave, err := s.enterOwner(true)
		if err != nil {
			if result != nil {
				result.Set(nil, cd.WrapError(cd.Unexpected, err, "governance is closing"))
			}
			return
		}
		defer leave()
		s.handleCommand(ev, result)
	}); err != nil {
		return err
	}
	s.mu.Lock()
	s.topics = append(s.topics, topic)
	s.mu.Unlock()
	if err := s.scheduleBlockedRunReconciliationLocked(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return basebiz.ErrLifecycleClosing
	}
	s.started = true
	return nil
}

func (s *Service) enterOwner(drain bool) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.releasing || s.released {
		return nil, basebiz.ErrLifecycleClosing
	}
	if drain {
		return s.lifecycle.EnterDrain()
	}
	return s.lifecycle.Enter()
}

func governanceTicketCreatedData(event magicevent.Event) (decisionevents.TicketChangedData, bool) {
	if event == nil || event.Source() != decisioncommon.ModuleID {
		return decisionevents.TicketChangedData{}, false
	}
	data, ok := event.Data().(decisionevents.TicketChangedData)
	return data, ok
}

func governanceRunStateChangedData(event magicevent.Event) (runevents.RunStateChangedData, bool) {
	if event == nil || event.Source() != runcommon.ModuleID {
		return runevents.RunStateChangedData{}, false
	}
	data, ok := event.Data().(runevents.RunStateChangedData)
	return data, ok
}

func (s *Service) BeginShutdown() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closing = true
	s.lifecycle.BeginShutdown()
	cancel, workCancel := s.reconcileCancel, s.workCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if workCancel != nil {
		workCancel()
	}
}

func (s *Service) Quiesce(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return basebiz.ErrNilLifecycleContext
	}
	if err := s.lifecycle.Quiesce(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	done, workDone := s.reconcileDone, s.workDone
	s.mu.Unlock()
	for _, ch := range []chan struct{}{done, workDone} {
		if ch != nil {
			select {
			case <-ch:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

// Stop 保留旧版必需生命周期助手合同；生产路径使用 StopChecked。
func (s *Service) Stop() {
	if err := s.StopChecked(context.Background()); err != nil {
		panic(cd.WrapError(cd.Unexpected, err, "governance shutdown failed"))
	}
}

func (s *Service) StopChecked(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.BeginShutdown()
	if err := s.Quiesce(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	if s.released {
		s.mu.Unlock()
		return nil
	}
	if s.releasing {
		s.mu.Unlock()
		return common.ErrConflict
	}
	s.releasing = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.releasing = false; s.mu.Unlock() }()
	if err := s.lifecycle.Quiesce(ctx); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		if len(s.topics) == 0 {
			s.mu.Unlock()
			break
		}
		topic := s.topics[len(s.topics)-1]
		s.mu.Unlock()
		if err := s.UnsubscribeFuncChecked(topic); err != nil {
			return err
		}
		s.mu.Lock()
		s.topics = s.topics[:len(s.topics)-1]
		s.mu.Unlock()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		if len(s.unsubscribes) == 0 {
			s.mu.Unlock()
			break
		}
		unsub := s.unsubscribes[len(s.unsubscribes)-1]
		s.mu.Unlock()
		if err := unsub(); err != nil {
			return err
		}
		s.mu.Lock()
		s.unsubscribes = s.unsubscribes[:len(s.unsubscribes)-1]
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.workCtx, s.workCancel, s.workQueue, s.workDone = nil, nil, nil, nil
	s.reconcileCancel, s.reconcileDone = nil, nil
	s.started, s.released = false, true
	s.mu.Unlock()
	return nil
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
	if s == nil || s.traces == nil || ev == nil || ev.Source() != "application/workorchd/appservice" {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	if ev.ID() != governanceevents.TopicGovernanceAutoCommandListTraces {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(governanceevents.ListAutoGovernanceTracesCommand)
	if !ok {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	cmd, err := cmd.Normalized()
	if err != nil {
		return governanceevents.GovernanceCommandResult{}, err
	}
	items, err := s.traces.List(ev.Context(), cmd.Query)
	if err != nil {
		return governanceevents.GovernanceCommandResult{}, err
	}
	return governanceevents.GovernanceCommandResult{Traces: items}, nil
}

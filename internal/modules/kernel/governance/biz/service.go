package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

var structuredValuePattern = regexp.MustCompile(`([A-Za-z0-9_\-\p{Han}]{2,32})\s*[:：=]\s*([^\s,，。;；]+)`)

const (
	recoveryPolicyRuntimeRepairExhausted = "runtime_repair_exhausted_requires_strategy"
)

type ResolveDecisionFunc func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error
type ResolveApprovalFunc func(context.Context, common.ApprovalID) error

type TicketReader interface {
	Get(context.Context, common.TicketID) (*decisionmodel.Ticket, error)
}

type ApprovalReader interface {
	Get(context.Context, common.ApprovalID) (*approvalmodel.Approval, error)
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
		s.SubscribeMatchedFunc(observerID, observerID, decisionevents.TopicTicketCreated, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(decisionevents.TicketChangedData); ok {
				s.enqueueGovernanceWork(func(workCtx context.Context) {
					s.processTicketCreated(workCtx, data)
				})
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
		unsubTicket = func() { s.UnsubscribeMatchedFunc(observerID, decisionevents.TopicTicketCreated) }
	}
	var unsubApproval func()
	if s.EventHub() != nil {
		const observerID = "/approval/consumers/kernel/governance"
		s.SubscribeMatchedFunc(observerID, observerID, approvalevents.TopicApprovalCreated, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(approvalevents.ApprovalChangedData); ok {
				s.enqueueGovernanceWork(func(workCtx context.Context) {
					s.processApprovalCreated(workCtx, data)
				})
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
		unsubApproval = func() { s.UnsubscribeMatchedFunc(observerID, approvalevents.TopicApprovalCreated) }
	}
	var unsubRunState func()
	if s.EventHub() != nil {
		const observerID = "/run/consumers/kernel/governance"
		s.SubscribeMatchedFunc(observerID, observerID, runevents.TopicRunStateChanged, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(runevents.RunStateChangedData); ok {
				s.enqueueGovernanceWork(func(workCtx context.Context) {
					s.processRunStateChanged(workCtx, data)
				})
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
		unsubRunState = func() { s.UnsubscribeMatchedFunc(observerID, runevents.TopicRunStateChanged) }
	}
	s.unsubscribes = []func(){unsubTicket, unsubApproval, unsubRunState}
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
	result.Set(response, nil)
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
	items, err := s.traces.List(ev.Context(), repository.AutoGovernanceTraceQuery{
		WorkspaceID: cmd.Query.WorkspaceID, RunID: cmd.Query.RunID, StepID: cmd.Query.StepID,
		ItemType: cmd.Query.ItemType, ItemID: cmd.Query.ItemID, Status: cmd.Query.Status, Limit: cmd.Query.Limit,
	})
	if err != nil {
		return governanceevents.GovernanceCommandResult{}, err
	}
	return governanceevents.GovernanceCommandResult{Traces: items}, nil
}

func (s *Service) startGovernanceWorker(ctx context.Context, queue <-chan governanceWork, done chan<- struct{}) error {
	if s.background != nil {
		return s.background.AsyncFunction(func() {
			s.runWorkLoop(ctx, queue, done)
		})
	}
	go s.runWorkLoop(ctx, queue, done)
	return nil
}

func (s *Service) runWorkLoop(ctx context.Context, queue <-chan governanceWork, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case work := <-queue:
			if work != nil {
				work(ctx)
			}
		}
	}
}

func (s *Service) enqueueGovernanceWork(work governanceWork) {
	if s == nil || work == nil {
		return
	}
	s.mu.Lock()
	ctx := s.workCtx
	queue := s.workQueue
	s.mu.Unlock()
	if ctx == nil || queue == nil {
		return
	}
	select {
	case queue <- work:
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *Service) scheduleBlockedRunReconciliationLocked() error {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.reconcileCancel = cancel
	s.reconcileDone = done
	reconcile := func() {
		defer close(done)
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			s.enqueueGovernanceWork(func(workCtx context.Context) {
				s.reconcileBlockedRuns(workCtx)
			})
		case <-ctx.Done():
			return
		}
	}
	if s.background != nil {
		return s.background.AsyncFunction(reconcile)
	}
	go reconcile()
	return nil
}

func (s *Service) processTicketCreated(ctx context.Context, data decisionevents.TicketChangedData) {
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, data.WorkspaceID)
	if err != nil {
		return
	}
	cfg, err := s.loadConfig()
	if err != nil || !isEnabled(cfg.Governance.AutoTicket.Enabled) {
		return
	}
	ticketID := common.TicketID(strings.TrimSpace(string(data.TicketID)))
	if ticketID == "" {
		return
	}
	s.processPendingTicket(ctx, ticketID)
}

func (s *Service) processPendingTicket(ctx context.Context, ticketID common.TicketID) {
	cfg, err := s.loadConfig()
	if err != nil || !isEnabled(cfg.Governance.AutoTicket.Enabled) {
		return
	}
	// A ticket ID is only locally unique.  A background/restart worker must
	// receive its workspace from an immutable owner fact before it may read or
	// resolve anything through another owner.
	if common.RuntimeWorkspaceIDFromContext(ctx) == "" {
		return
	}
	if ticketID == "" || s.hasClosedTrace(ctx, "ticket", string(ticketID)) {
		return
	}
	ticket, err := s.tickets.Get(ctx, ticketID)
	if err != nil || ticket == nil || ticket.State != common.DecisionTicketStateOpen {
		return
	}
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, ticket.WorkspaceID)
	if err != nil {
		return
	}
	run, err := s.runs.GetByWorkspaceID(ctx, ticket.WorkspaceID, ticket.RunID)
	if err != nil || run == nil {
		return
	}
	goal, dod := s.loadGoalAndDoD(ctx, run.GoalID, run.DoDID)
	memoryItems := s.loadMemoryItems(ctx, ticket.WorkspaceID)
	eval := s.evaluateTicket(ctx, ticket, run, goal, dod, memoryItems)
	trace := &governancemodel.AutoGovernanceTrace{
		ID:                 common.AutoGovernanceTraceID(fmt.Sprintf("%s/auto-governance", ticket.ID)),
		WorkspaceID:        ticket.WorkspaceID,
		RunID:              ticket.RunID,
		StepID:             ticket.StepID,
		ItemType:           "ticket",
		ItemID:             string(ticket.ID),
		Action:             "escalate",
		RiskLevel:          "L1",
		Confidence:         eval.confidence,
		PolicyMatched:      "",
		PolicyBlocked:      eval.policyBlocked,
		BudgetBefore:       eval.budgetBefore,
		BudgetAfter:        eval.budgetAfter,
		Reason:             eval.reason,
		EvidenceRefs:       eval.evidenceRefs,
		PromptFingerprint:  eval.promptKey,
		LLMProviderID:      "",
		LLMModel:           "",
		LLMDecisionPreview: eval.promptKey,
		Status:             "evaluated",
		OccurredAt:         nowUTC(),
	}
	_ = s.traces.Upsert(ctx, trace)
	_ = s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEvaluated, trace, "")

	if !eval.canAutoResolve {
		trace.Status = "escalated"
		trace.Action = "escalate"
		trace.OccurredAt = nowUTC()
		_ = s.traces.Upsert(ctx, trace)
		_ = s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEscalated, trace, eval.policyBlocked)
		return
	}
	payload := workorcheventpayload.New(
		workorcheventpayload.StringField("answer", eval.answer),
		workorcheventpayload.StringField("source", "auto_governance"),
		workorcheventpayload.FloatField("confidence", eval.confidence),
		workorcheventpayload.StringField("reason", eval.reason),
		workorcheventpayload.StringField(events.PayloadKeyWorkspaceID, string(ticket.WorkspaceID)),
		workorcheventpayload.StringField(events.PayloadKeyRunID, string(ticket.RunID)),
		workorcheventpayload.StringField(events.PayloadKeyStepID, string(ticket.StepID)),
	).Merge(eval.resolutionPayload)
	semantic := governancecommon.NormalizeDecisionResolution("", payload)
	payload = payload.Merge(workorcheventpayload.New(workorcheventpayload.ObjectField("governanceSemantic", workorcheventpayload.New(
		workorcheventpayload.StringField("resolutionType", semantic.ResolutionType),
		workorcheventpayload.StringField("intent", semantic.Intent),
		workorcheventpayload.FloatField("confidence", semantic.Confidence),
		workorcheventpayload.StringField("rawInput", semantic.RawInput),
		workorcheventpayload.StringField("source", semantic.Source),
		workorcheventpayload.BoolField("inferred", semantic.Inferred),
		workorcheventpayload.StringField("version", "v1"),
	))))
	if err := s.resolve(ctx, ticket.ID, semantic.ResolutionType, payload); err != nil {
		trace.Status = "blocked"
		trace.Action = "escalate"
		trace.PolicyBlocked = "resolve_failed"
		trace.Reason = err.Error()
		trace.OccurredAt = nowUTC()
		_ = s.traces.Upsert(ctx, trace)
		_ = s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoBlocked, trace, "resolve_failed")
		return
	}
	trace.Status = "resolved"
	trace.Action = "answer"
	trace.PolicyMatched = eval.policyMatched
	trace.PolicyBlocked = ""
	trace.BudgetAfter = eval.budgetAfter
	trace.Reason = eval.reason
	trace.OccurredAt = nowUTC()
	_ = s.traces.Upsert(ctx, trace)
	_ = s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoResolved, trace, "")

}

func (s *Service) processApprovalCreated(ctx context.Context, data approvalevents.ApprovalChangedData) {
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, data.WorkspaceID)
	if err != nil {
		return
	}
	cfg, err := s.loadConfig()
	if err != nil || !isEnabled(cfg.Governance.AutoApproval.Enabled) {
		return
	}
	approvalID := common.ApprovalID(strings.TrimSpace(string(data.ApprovalID)))
	if approvalID == "" {
		return
	}
	s.processPendingApproval(ctx, approvalID)
}

func (s *Service) processPendingApproval(ctx context.Context, approvalID common.ApprovalID) {
	cfg, err := s.loadConfig()
	if err != nil || !isEnabled(cfg.Governance.AutoApproval.Enabled) {
		return
	}
	// Approval IDs also require workspace scope.  Do not let a process-global
	// reconciliation loop turn an unscoped identifier into owner I/O.
	if common.RuntimeWorkspaceIDFromContext(ctx) == "" {
		return
	}
	if approvalID == "" || s.hasClosedTrace(ctx, "approval", string(approvalID)) {
		return
	}
	approval, err := s.approvals.Get(ctx, approvalID)
	if err != nil || approval == nil || approval.State != common.ApprovalStatePending {
		return
	}
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, approval.WorkspaceID)
	if err != nil {
		return
	}
	run, err := s.runs.GetByWorkspaceID(ctx, approval.WorkspaceID, approval.RunID)
	if err != nil || run == nil {
		return
	}
	goal, dod := s.loadGoalAndDoD(ctx, run.GoalID, run.DoDID)
	eval := s.evaluateApproval(ctx, approval, goal, dod, cfg.Governance)
	trace := &governancemodel.AutoGovernanceTrace{
		ID:                 common.AutoGovernanceTraceID(fmt.Sprintf("%s/auto-governance", approval.ID)),
		WorkspaceID:        approval.WorkspaceID,
		RunID:              approval.RunID,
		StepID:             approval.StepID,
		ItemType:           "approval",
		ItemID:             string(approval.ID),
		Action:             "escalate",
		RiskLevel:          eval.riskLevel,
		Confidence:         eval.confidence,
		PolicyMatched:      "",
		PolicyBlocked:      eval.policyBlocked,
		BudgetBefore:       eval.budgetBefore,
		BudgetAfter:        eval.budgetAfter,
		Reason:             eval.reason,
		EvidenceRefs:       eval.evidenceRefs,
		PromptFingerprint:  eval.promptKey,
		LLMProviderID:      "",
		LLMModel:           "",
		LLMDecisionPreview: eval.promptKey,
		Status:             "evaluated",
		OccurredAt:         nowUTC(),
	}
	_ = s.traces.Upsert(ctx, trace)
	_ = s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEvaluated, trace, "")

	if !eval.canAutoApprove {
		trace.Status = "escalated"
		trace.Action = "escalate"
		trace.OccurredAt = nowUTC()
		_ = s.traces.Upsert(ctx, trace)
		_ = s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEscalated, trace, eval.policyBlocked)
		return
	}

	if err := s.resolveApproval(ctx, approval.ID); err != nil {
		trace.Status = "blocked"
		trace.Action = "escalate"
		trace.PolicyBlocked = "resolve_failed"
		trace.Reason = err.Error()
		trace.OccurredAt = nowUTC()
		_ = s.traces.Upsert(ctx, trace)
		_ = s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoBlocked, trace, "resolve_failed")
		return
	}

	trace.Status = "resolved"
	trace.Action = "approve"
	trace.PolicyMatched = eval.policyMatched
	trace.PolicyBlocked = ""
	trace.BudgetAfter = eval.budgetAfter
	trace.Reason = eval.reason
	trace.OccurredAt = nowUTC()
	_ = s.traces.Upsert(ctx, trace)
	_ = s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoResolved, trace, "")

}

func (s *Service) processRunStateChanged(ctx context.Context, data runevents.RunStateChangedData) {
	if data.Validate() != nil {
		return
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, data.WorkspaceID)
	if err != nil {
		return
	}
	if data.State == common.RunStateBlockedOnDecision && data.BlockingTicketID != "" {
		ticket, err := s.tickets.Get(ctx, data.BlockingTicketID)
		if err != nil || ticket == nil {
			return
		}
		if ticket.State == common.DecisionTicketStateOpen {
			s.processPendingTicket(ctx, ticket.ID)
			return
		}
		if ticket.State != common.DecisionTicketStateResolved {
			return
		}
		// A resolved ticket is owner evidence. Runtime V2 continuation is solely
		// scheduled by the Orchestrator when it consumes that typed evidence.
		return
	}
	if data.State == common.RunStateBlockedOnApproval && data.BlockingApprovalID != "" {
		approval, err := s.approvals.Get(ctx, data.BlockingApprovalID)
		if err != nil || approval == nil {
			return
		}
		if approval.State == common.ApprovalStatePending {
			s.processPendingApproval(ctx, approval.ID)
			return
		}
		if approval.State != common.ApprovalStateApproved {
			return
		}
		// An approved record is owner evidence. Governance never drives a Run.
	}
}

func (s *Service) reconcileBlockedRuns(ctx context.Context) {
	if s == nil || s.runs == nil {
		return
	}
	runs, err := s.runs.List(ctx)
	if err != nil {
		return
	}
	for _, item := range runs {
		if item == nil || item.WorkspaceID == "" {
			continue
		}
		workspaceCtx, scopeErr := common.BindRuntimeWorkspaceContext(ctx, item.WorkspaceID)
		if scopeErr != nil {
			continue
		}
		switch item.State {
		case common.RunStateBlockedOnDecision:
			if item.BlockingTicketID != "" {
				s.processPendingTicket(workspaceCtx, item.BlockingTicketID)
			}
		case common.RunStateBlockedOnApproval:
			if item.BlockingApprovalID != "" {
				s.processPendingApproval(workspaceCtx, item.BlockingApprovalID)
			}
		}
	}
}

type ticketEvaluation struct {
	canAutoResolve    bool
	answer            string
	confidence        float64
	reason            string
	policyBlocked     string
	policyMatched     string
	budgetBefore      int
	budgetAfter       int
	promptKey         string
	evidenceRefs      []string
	resolutionPayload workorcheventpayload.Payload
}

type approvalEvaluation struct {
	canAutoApprove bool
	riskLevel      string
	confidence     float64
	reason         string
	policyMatched  string
	policyBlocked  string
	budgetBefore   int
	budgetAfter    int
	promptKey      string
	evidenceRefs   []string
}

func (s *Service) evaluateTicket(ctx context.Context, ticket *decisionmodel.Ticket, run *runmodel.Run, goal *workspace.Goal, dod *workspace.DoD, memoryItems []*memorymodel.MemoryItem) ticketEvaluation {
	ret := ticketEvaluation{
		canAutoResolve: false,
		confidence:     0.0,
		reason:         "insufficient context for auto decision",
		policyBlocked:  "insufficient_context",
		policyMatched:  "",
		evidenceRefs:   []string{string(ticket.ID)},
	}
	if ticket == nil || ticket.ID == "" {
		ret.policyBlocked = "invalid_ticket"
		return ret
	}
	decisionKind := strings.TrimSpace(ticket.DecisionKind)
	if !decisioncommon.IsSupportedResolutionType(decisionKind) {
		ret.policyBlocked = "unsupported_decision_kind"
		return ret
	}

	cfg, err := s.loadConfig()
	if err != nil {
		ret.policyBlocked = "load_config_failed"
		return ret
	}
	ret.promptKey = compactPromptKey(ticket.Question + "\n" + ticket.Reason)
	budgetUsed := s.countResolvedTicketTraces(ctx, ticket.RunID)
	ret.budgetBefore = budgetUsed
	maxPerRun := cfg.Governance.AutoTicket.MaxPerRun
	if maxPerRun <= 0 {
		maxPerRun = 5
	}
	if budgetUsed >= maxPerRun {
		ret.policyBlocked = "budget_exhausted"
		ret.reason = "auto ticket budget exhausted"
		return ret
	}

	question := strings.TrimSpace(ticket.Question)
	reason := strings.TrimSpace(ticket.Reason)
	if blockedReason, ok := s.blockedBySharedBudgetPolicy(ctx, cfg.Governance, ticket.RunID, "ticket", string(ticket.ID), ret.promptKey); ok {
		ret.policyBlocked = blockedReason
		ret.reason = "auto governance budget policy blocked ticket resolution"
		return ret
	}
	if handled, recoveryEval := s.evaluateRuntimeRepairExhaustionTicket(ctx, ticket, run, ret); handled {
		if recoveryEval.canAutoResolve {
			recoveryEval.budgetAfter = budgetUsed + 1
			if goal != nil && goal.ID != "" {
				recoveryEval.evidenceRefs = append(recoveryEval.evidenceRefs, string(goal.ID))
			}
		}
		return recoveryEval
	}
	switch decisionKind {
	case decisioncommon.ResolutionTypeProvideMissingInfo:
		if requiresHumanByQuestion(question, reason) {
			ret.policyBlocked = "human_confirmation_required"
			ret.reason = "ticket requires explicit human confirmation"
			return ret
		}
		goalText := ""
		if goal != nil {
			goalText = strings.TrimSpace(goal.Text)
		}
		answer, confidence, answerReason, evidenceRefs := inferMissingInfoAnswer(ticket.Context, question, reason, goalText, dod, memoryItems)
		minConfidence := cfg.Governance.AutoTicket.MinConfidence
		if minConfidence <= 0 {
			minConfidence = 0.82
		}
		if strings.TrimSpace(answer) == "" || confidence < minConfidence {
			ret.policyBlocked = "low_confidence"
			ret.confidence = confidence
			ret.reason = "auto answer confidence below threshold"
			return ret
		}
		ret.canAutoResolve = true
		ret.answer = answer
		ret.confidence = confidence
		ret.reason = answerReason
		ret.policyBlocked = ""
		ret.policyMatched = "goal_dod_memory_structured_inference"
		ret.budgetAfter = budgetUsed + 1
		if goal != nil && goal.ID != "" {
			ret.evidenceRefs = append(ret.evidenceRefs, string(goal.ID))
		}
		ret.evidenceRefs = append(ret.evidenceRefs, evidenceRefs...)
		return ret
	case decisioncommon.ResolutionTypeReplan:
		if hasManualRequiredMarker(ticket.Context) {
			ret.policyBlocked = "manual_replan_required"
			ret.reason = "replan requires explicit human decision fields"
			return ret
		}
		if runtimeRecoveryRequiresExternalOperator(ticket.Context, question, reason) {
			ret.policyBlocked = "external_recovery_required"
			ret.reason = "runtime recovery requires external provider or configuration action"
			return ret
		}
		missingFields := stringSliceFromMap(ticket.Context, "missingFields")
		if len(missingFields) > 0 {
			ret.policyBlocked = "missing_replan_fields"
			ret.reason = "replan ticket still has unresolved required fields"
			return ret
		}
		allowedActions := stringSliceFromMap(ticket.Context, "allowedRecoveryActions")
		if len(allowedActions) > 0 && !stringSliceContainsFold(allowedActions, "replan") {
			ret.policyBlocked = "replan_not_allowed"
			ret.reason = "runtime recovery policy does not allow auto replan"
			return ret
		}
		ret.canAutoResolve = true
		ret.answer = "replan"
		ret.confidence = 0.9
		ret.reason = "ticket decision kind is replan and no manual fields remain"
		ret.policyBlocked = ""
		ret.policyMatched = "decision_kind_replan_auto_resolve"
		ret.budgetAfter = budgetUsed + 1
		ret.resolutionPayload = workorcheventpayload.New(workorcheventpayload.StringField("recoveryAction", "replan"))
		if goal != nil && goal.ID != "" {
			ret.evidenceRefs = append(ret.evidenceRefs, string(goal.ID))
		}
		return ret
	default:
		ret.policyBlocked = "manual_decision_kind"
		ret.reason = "decision kind requires manual governance"
		return ret
	}
}

func (s *Service) evaluateRuntimeRepairExhaustionTicket(ctx context.Context, ticket *decisionmodel.Ticket, run *runmodel.Run, base ticketEvaluation) (bool, ticketEvaluation) {
	if ticket == nil || strings.TrimSpace(stringFromMap(ticket.Context, "recoveryPolicy")) != recoveryPolicyRuntimeRepairExhausted {
		return false, base
	}
	ret := base
	ret.policyBlocked = "runtime_repair_exhausted_requires_strategy"
	ret.reason = "runtime repair exhaustion requires an explicit recovery strategy"
	if hasManualRequiredMarker(ticket.Context) {
		ret.policyBlocked = "manual_recovery_required"
		ret.reason = "runtime recovery context requires manual input"
		return true, ret
	}
	if runtimeRecoveryRequiresExternalOperator(ticket.Context, ticket.Question, ticket.Reason) {
		ret.policyBlocked = "external_recovery_required"
		ret.reason = "runtime recovery requires external provider or configuration action"
		return true, ret
	}
	if !stringSliceContainsFold(stringSliceFromMap(ticket.Context, "allowedRecoveryActions"), "replan") {
		ret.policyBlocked = "replan_not_allowed"
		ret.reason = "runtime recovery policy does not allow auto replan"
		return true, ret
	}
	if s != nil && s.autoReplanLineageDepth(ctx, run) > 0 {
		ret.policyBlocked = "replan_lineage_budget_exhausted"
		ret.reason = "runtime recovery already used automatic replan in this lineage"
		return true, ret
	}
	ret.canAutoResolve = true
	ret.answer = "replan"
	ret.confidence = 0.91
	ret.reason = "runtime repair exhausted and structured recovery policy allows replan"
	ret.policyBlocked = ""
	ret.policyMatched = "runtime_repair_exhausted_auto_replan"
	ret.resolutionPayload = workorcheventpayload.New(workorcheventpayload.StringField("recoveryAction", "replan"))
	return true, ret
}

func (s *Service) autoReplanLineageDepth(ctx context.Context, run *runmodel.Run) int {
	if s == nil || s.runs == nil || run == nil || run.ID == "" {
		return 0
	}
	depth := 0
	seen := map[common.RunID]bool{}
	for current := run; current != nil && current.ID != "" && !seen[current.ID]; {
		seen[current.ID] = true
		if current.RuntimeBootstrap == nil {
			return depth
		}
		replan := current.RuntimeBootstrap.Activation.Continuation.Normalized().Replan
		if replan == nil {
			return depth
		}
		if strings.EqualFold(replan.ResolutionType, decisioncommon.ResolutionTypeReplan) || strings.EqualFold(replan.RecoveryAction, "replan") {
			depth++
		}
		predecessorID := replan.PredecessorRunID
		if predecessorID == "" {
			return depth
		}
		predecessor, err := s.runs.Get(ctx, predecessorID)
		if err != nil || predecessor == nil {
			return depth
		}
		current = predecessor
	}
	return depth
}

func (s *Service) evaluateApproval(ctx context.Context, approval *approvalmodel.Approval, goal *workspace.Goal, dod *workspace.DoD, governanceCfg workorchconfig.GovernanceConfig) approvalEvaluation {
	ret := approvalEvaluation{
		canAutoApprove: false,
		riskLevel:      "L3",
		confidence:     0,
		reason:         "approval requires manual review",
		policyMatched:  "",
		policyBlocked:  "policy_manual_required",
		evidenceRefs:   []string{string(approval.ID)},
	}
	if approval == nil || approval.ID == "" {
		ret.policyBlocked = "invalid_approval"
		ret.reason = "invalid approval item"
		return ret
	}
	if approval.State != common.ApprovalStatePending {
		ret.policyBlocked = "approval_not_pending"
		ret.reason = "approval already resolved"
		return ret
	}
	ret.promptKey = compactPromptKey(approval.Action + "\n" + approval.Reason + "\n" + stableContextDigest(approval.Context))
	resolvedCount := s.countResolvedApprovalTraces(ctx, approval.RunID)
	ret.budgetBefore = resolvedCount
	maxPerRun := governanceCfg.AutoApproval.MaxPerRun
	if maxPerRun <= 0 {
		maxPerRun = 3
	}
	if resolvedCount >= maxPerRun {
		ret.policyBlocked = "budget_exhausted"
		ret.reason = "auto approval budget exhausted"
		return ret
	}
	if blockedReason, ok := s.blockedBySharedBudgetPolicy(ctx, governanceCfg, approval.RunID, "approval", string(approval.ID), ret.promptKey); ok {
		ret.policyBlocked = blockedReason
		ret.reason = "auto governance budget policy blocked approval resolution"
		return ret
	}

	riskLevel, riskSignals := classifyApprovalRisk(approval)
	ret.riskLevel = riskLevel
	ret.evidenceRefs = append(ret.evidenceRefs, riskSignals...)
	if requiresHumanByPolicy(riskSignals, governanceCfg.AutoApproval.RequireHumanFor) {
		ret.policyBlocked = "require_human_policy_matched"
		ret.reason = "approval matched requireHumanFor policy"
		return ret
	}
	if !withinDoDBoundary(approval, goal, dod) {
		ret.policyBlocked = "dod_mismatch"
		ret.reason = "approval is outside current goal/DoD scope"
		return ret
	}
	switch riskLevel {
	case "L1":
		if !isEnabled(governanceCfg.AutoApproval.AllowReadOnly) {
			ret.policyBlocked = "read_only_auto_disabled"
			ret.reason = "read-only auto approval disabled by policy"
			return ret
		}
		ret.canAutoApprove = true
		ret.policyMatched = "read_only"
		ret.confidence = 0.96
		ret.reason = "read-only approval auto-resolved by policy"
	case "L2":
		if !isEnabled(governanceCfg.AutoApproval.AllowWorkspaceWrite) {
			ret.policyBlocked = "workspace_write_auto_disabled"
			ret.reason = "workspace write auto approval disabled by policy"
			return ret
		}
		ret.canAutoApprove = true
		ret.policyMatched = "workspace_low_risk_write"
		ret.confidence = 0.9
		ret.reason = "workspace low-risk write auto-approved by policy"
	default:
		ret.policyBlocked = "risk_level_requires_manual"
		ret.reason = "risk level requires manual approval"
		return ret
	}
	minConfidence := governanceCfg.AutoApproval.MinConfidence
	if minConfidence <= 0 {
		minConfidence = 0.9
	}
	if ret.confidence < minConfidence {
		ret.canAutoApprove = false
		ret.policyMatched = ""
		ret.policyBlocked = "low_confidence"
		ret.reason = "auto approval confidence below threshold"
		return ret
	}
	ret.budgetAfter = resolvedCount + 1
	return ret
}

func inferMissingInfoAnswer(
	ctxMap workorcheventpayload.Payload,
	question string,
	reason string,
	goalText string,
	dod *workspace.DoD,
	memoryItems []*memorymodel.MemoryItem,
) (string, float64, string, []string) {
	evidenceRefs := []string{}
	if value := firstNonEmptyMapValue(ctxMap, []string{"answer", "value", "target", "input", "selection", "requested", "default", "path", "command", "url", "branch", "language"}); value != "" {
		return value, 0.97, "answer inferred from structured ticket context", evidenceRefs
	}
	if value := singleStringMapValue(ctxMap); value != "" {
		return value, 0.9, "answer inferred from single ticket context value", evidenceRefs
	}
	if value, confidence, ok := structuredCandidateAnswer(ctxMap); ok {
		return value, confidence, "answer inferred from structured ticket candidates", evidenceRefs
	}
	if value, confidence, ok := continuationCandidateAnswer(ctxMap); ok {
		return value, confidence, "answer inferred from continuable ticket candidate", evidenceRefs
	}
	if hasCandidateValues(ctxMap) {
		return "", 0, "", evidenceRefs
	}

	query := normalize(strings.Join([]string{question, reason}, " "))
	if value, ok := findStructuredValueInText(query, goalText); ok {
		return value, 0.9, "answer inferred from goal text", evidenceRefs
	}
	if dod != nil {
		for _, item := range dod.Items {
			if value, ok := findStructuredValueInText(query, item); ok {
				return value, 0.87, "answer inferred from DoD items", evidenceRefs
			}
		}
	}
	for i := len(memoryItems) - 1; i >= 0; i-- {
		item := memoryItems[i]
		if item == nil {
			continue
		}
		text := strings.TrimSpace(strings.Join([]string{item.Content, item.Summary, item.EmbeddingText}, " "))
		if value, ok := findStructuredValueInText(query, text); ok {
			evidence := string(item.ID)
			if evidence != "" {
				evidenceRefs = append(evidenceRefs, evidence)
			}
			return value, 0.85, "answer inferred from workspace memory", evidenceRefs
		}
	}
	return "", 0.0, "", evidenceRefs
}

func hasCandidateValues(input workorcheventpayload.Payload) bool {
	_, ok := input.Field("candidateValues")
	return ok
}

func continuationCandidateAnswer(input workorcheventpayload.Payload) (string, float64, bool) {
	candidates, ok := input.Object("candidateValues")
	if !ok || len(candidates.Fields) == 0 {
		return "", 0, false
	}
	confidences := confidenceMapFromMap(input, "confidenceByField")
	selected := map[string]string{}
	confidence := 1.0
	confidenceSeen := false
	for _, fieldValue := range candidates.Fields {
		field := strings.TrimSpace(fieldValue.Name)
		if field == "" {
			continue
		}
		candidate := preferredAutoContinuationCandidate(fieldValue.Value)
		if candidate == "" {
			continue
		}
		selected[field] = candidate
		if fieldConfidence, ok := confidences[normalize(field)]; ok {
			confidence = minFloat(confidence, fieldConfidence)
			confidenceSeen = true
		}
	}
	if len(selected) == 0 {
		return "", 0, false
	}
	if !confidenceSeen {
		confidence = 0.9
	}
	answer := joinStructuredAnswer(selected, sortedMapKeys(selected))
	return answer, confidence, answer != ""
}

func preferredAutoContinuationCandidate(value workorcheventpayload.Value) string {
	switch value.Kind {
	case workorcheventpayload.ValueString:
		if isAutoContinuationCandidateText(value.String) {
			return strings.TrimSpace(value.String)
		}
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			if candidate := preferredAutoContinuationCandidate(item); candidate != "" {
				return candidate
			}
		}
	case workorcheventpayload.ValueObject:
		object := workorcheventpayload.Payload{Fields: value.Object}
		for _, key := range []string{"value", "id", "mode", "target", "name"} {
			if nested, ok := object.Field(key); ok {
				if candidate := preferredAutoContinuationCandidate(nested); candidate != "" {
					return candidate
				}
			}
		}
		for _, nested := range value.Object {
			if candidate := preferredAutoContinuationCandidate(nested.Value); candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

func isAutoContinuationCandidateText(value string) bool {
	normalized := normalizeContinuationCandidateText(value)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"init", "initialize", "bootstrap", "fromscratch", "newscratch", "newproject",
		"currentworkspace", "currentdirectory",
		"初始化", "从零", "新项目", "当前工作区", "当前目录",
	} {
		if strings.Contains(normalized, normalizeContinuationCandidateText(marker)) {
			return true
		}
	}
	return false
}

func normalizeContinuationCandidateText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "_", "", "/", "", "\\", "", ".", "")
	return replacer.Replace(value)
}

func structuredCandidateAnswer(input workorcheventpayload.Payload) (string, float64, bool) {
	if boolFromMap(input, "humanRequired") ||
		boolFromMap(input, "requiresHuman") ||
		boolFromMap(input, "manualRequired") ||
		len(stringSliceFromMap(input, "requiresHumanFields")) > 0 {
		return "", 0, false
	}
	knownValues := stringMapFromMap(input, "knownValues")
	candidateValues := stringMapFromMap(input, "candidateValues")
	resolvedValues := stringMapFromMap(input, "resolvedValues")
	missingFields := stringSliceFromMap(input, "missingFields")
	confidenceByField := confidenceMapFromMap(input, "confidenceByField")
	topConfidenceValue, hasTopConfidence := input.Field("confidence")
	topConfidence, hasTopConfidence := confidenceValue(topConfidenceValue, hasTopConfidence)

	values := make(map[string]string, len(knownValues)+len(candidateValues)+len(resolvedValues))
	for key, value := range knownValues {
		values[key] = value
	}
	for key, value := range candidateValues {
		values[key] = value
	}
	for key, value := range resolvedValues {
		values[key] = value
	}
	if len(values) == 0 {
		return "", 0, false
	}
	if len(missingFields) > 0 {
		for _, field := range missingFields {
			normalized := normalize(field)
			if normalized == "" {
				continue
			}
			if _, ok := mapValueCaseInsensitive(candidateValues, normalized); !ok {
				if _, ok := mapValueCaseInsensitive(resolvedValues, normalized); !ok {
					return "", 0, false
				}
			}
		}
	}

	confidence := 1.0
	confidenceSeen := false
	for key := range candidateValues {
		if value, ok := confidenceByField[normalize(key)]; ok {
			confidence = minFloat(confidence, value)
			confidenceSeen = true
		}
	}
	for key := range resolvedValues {
		if value, ok := confidenceByField[normalize(key)]; ok {
			confidence = minFloat(confidence, value)
			confidenceSeen = true
		}
	}
	if !confidenceSeen {
		if !hasTopConfidence {
			return "", 0, false
		}
		confidence = topConfidence
	}

	answer := joinStructuredAnswer(values, append(missingFields, sortedMapKeys(values)...))
	if answer == "" {
		return "", 0, false
	}
	return answer, confidence, true
}

func joinStructuredAnswer(values map[string]string, preferredOrder []string) string {
	if len(values) == 0 {
		return ""
	}
	seen := map[string]bool{}
	parts := make([]string, 0, len(values))
	appendField := func(field string) {
		key := normalize(field)
		if key == "" || seen[key] {
			return
		}
		value, ok := mapValueCaseInsensitive(values, key)
		if !ok {
			return
		}
		if value == "" {
			return
		}
		seen[key] = true
		parts = append(parts, strings.TrimSpace(field)+"="+value)
	}
	for _, key := range preferredOrder {
		appendField(key)
	}
	for _, key := range sortedMapKeys(values) {
		appendField(key)
	}
	return strings.Join(parts, ", ")
}

func mapValueCaseInsensitive(input map[string]string, normalizedKey string) (string, bool) {
	for key, value := range input {
		if normalize(key) == normalizedKey {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func sortedMapKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func findStructuredValueInText(query string, text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	matches := structuredValuePattern.FindAllStringSubmatch(text, -1)
	for _, pair := range matches {
		if len(pair) < 3 {
			continue
		}
		key := normalize(pair[1])
		value := strings.TrimSpace(pair[2])
		if key == "" || value == "" {
			continue
		}
		if strings.Contains(query, key) {
			return value, true
		}
	}
	return "", false
}

func requiresHumanByQuestion(question string, reason string) bool {
	combined := normalize(question + " " + reason)
	if strings.Contains(combined, "是否") ||
		strings.Contains(combined, "approve") ||
		strings.Contains(combined, "批准") ||
		strings.Contains(combined, "拒绝") {
		return true
	}
	return false
}

func hasManualRequiredMarker(input workorcheventpayload.Payload) bool {
	return boolFromMap(input, "humanRequired") ||
		boolFromMap(input, "requiresHuman") ||
		boolFromMap(input, "manualRequired") ||
		len(stringSliceFromMap(input, "requiresHumanFields")) > 0
}

func runtimeRecoveryRequiresExternalOperator(ctxMap workorcheventpayload.Payload, question string, reason string) bool {
	scope := normalize(stringFromMap(ctxMap, "scope"))
	dependency := normalize(stringFromMap(ctxMap, "dependency"))
	policy := normalize(stringFromMap(ctxMap, "recoveryPolicy"))
	failureClass := normalize(stringFromMap(ctxMap, "failureClass"))
	if scope == "workorch_runtime" && dependency == "llm_provider" {
		return true
	}
	if policy == "system_config_required" || policy == "config_only" {
		return true
	}
	if strings.HasPrefix(failureClass, "provider_") ||
		failureClass == "context_too_large" ||
		failureClass == "rate_limited" ||
		failureClass == "auth_or_config_error" {
		return true
	}
	text := normalize(question + "\n" + reason)
	return strings.Contains(text, "llm provider") || strings.Contains(text, "provider retry")
}

func stringSliceContainsFold(values []string, want string) bool {
	want = normalize(want)
	for _, value := range values {
		if normalize(value) == want {
			return true
		}
	}
	return false
}

func normalize(input string) string {
	return strings.ToLower(strings.TrimSpace(input))
}

func firstNonEmptyMapValue(input workorcheventpayload.Payload, keys []string) string {
	for _, key := range keys {
		value := strings.TrimSpace(stringFromMap(input, key))
		if value != "" {
			return value
		}
	}
	return ""
}

func singleStringMapValue(input workorcheventpayload.Payload) string {
	if len(input.Fields) != 1 {
		return ""
	}
	field := input.Fields[0]
	if field.Value.Kind != workorcheventpayload.ValueString {
		return ""
	}
	return strings.TrimSpace(field.Value.String)
}

func stringMapFromMap(input workorcheventpayload.Payload, key string) map[string]string {
	ret := map[string]string{}
	object, ok := input.Object(key)
	if !ok {
		return ret
	}
	for _, field := range object.Fields {
		if value := stringifyScalar(field.Value); value != "" {
			ret[strings.TrimSpace(field.Name)] = value
		}
	}
	return ret
}

func confidenceMapFromMap(input workorcheventpayload.Payload, key string) map[string]float64 {
	ret := map[string]float64{}
	object, ok := input.Object(key)
	if !ok {
		return ret
	}
	for _, field := range object.Fields {
		if value, ok := confidenceValue(field.Value, true); ok {
			ret[normalize(field.Name)] = value
		}
	}
	return ret
}

func stringSliceFromMap(input workorcheventpayload.Payload, key string) []string {
	values, ok := input.StringSlice(key)
	if !ok {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func boolFromMap(input workorcheventpayload.Payload, key string) bool {
	value, ok := input.Field(key)
	if !ok {
		return false
	}
	switch value.Kind {
	case workorcheventpayload.ValueBool:
		return value.Bool
	case workorcheventpayload.ValueString:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value.String))
		return err == nil && parsed
	}
	return false
}

func confidenceValue(input workorcheventpayload.Value, present bool) (float64, bool) {
	if !present {
		return 0, false
	}
	switch input.Kind {
	case workorcheventpayload.ValueFloat:
		return input.Float, true
	case workorcheventpayload.ValueInt:
		return float64(input.Int), true
	case workorcheventpayload.ValueUint:
		return float64(input.Uint), true
	case workorcheventpayload.ValueString:
		switch normalize(input.String) {
		case "high":
			return 0.9, true
		case "medium":
			return 0.75, true
		case "low":
			return 0.5, true
		default:
			value, err := strconv.ParseFloat(strings.TrimSpace(input.String), 64)
			return value, err == nil
		}
	}
	return 0, false
}

func stringifyScalar(input workorcheventpayload.Value) string {
	switch input.Kind {
	case workorcheventpayload.ValueString:
		return strings.TrimSpace(input.String)
	case workorcheventpayload.ValueFloat:
		return strconv.FormatFloat(input.Float, 'g', -1, 64)
	case workorcheventpayload.ValueInt:
		return strconv.FormatInt(input.Int, 10)
	case workorcheventpayload.ValueUint:
		return strconv.FormatUint(input.Uint, 10)
	case workorcheventpayload.ValueBool:
		return strconv.FormatBool(input.Bool)
	}
	return ""
}

func minFloat(a float64, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func stringFromMap(input workorcheventpayload.Payload, key string) string {
	value, _ := input.String(key)
	return strings.TrimSpace(value)
}

func (s *Service) countResolvedTicketTraces(ctx context.Context, runID common.RunID) int {
	if s == nil || s.traces == nil || runID == "" {
		return 0
	}
	list, err := s.traces.List(ctx, repository.AutoGovernanceTraceQuery{RunID: runID, ItemType: "ticket"})
	if err != nil {
		return 0
	}
	count := 0
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, "resolved") && strings.EqualFold(item.Action, "answer") {
			count++
		}
	}
	return count
}

func (s *Service) countResolvedApprovalTraces(ctx context.Context, runID common.RunID) int {
	if s == nil || s.traces == nil || runID == "" {
		return 0
	}
	list, err := s.traces.List(ctx, repository.AutoGovernanceTraceQuery{RunID: runID, ItemType: "approval"})
	if err != nil {
		return 0
	}
	count := 0
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, "resolved") && strings.EqualFold(item.Action, "approve") {
			count++
		}
	}
	return count
}

func (s *Service) blockedBySharedBudgetPolicy(ctx context.Context, governanceCfg workorchconfig.GovernanceConfig, runID common.RunID, itemType string, currentItemID string, promptKey string) (string, bool) {
	traces, err := s.loadTracesByRunAndType(ctx, runID, itemType)
	if err != nil {
		return "", false
	}
	maxConsecutiveFailures := governanceCfg.Budget.MaxConsecutiveFailures
	if maxConsecutiveFailures > 0 {
		consecutive := 0
		for i := len(traces) - 1; i >= 0; i-- {
			item := traces[i]
			if item == nil {
				continue
			}
			if strings.TrimSpace(item.ItemID) == strings.TrimSpace(currentItemID) {
				continue
			}
			if strings.EqualFold(item.Status, "resolved") {
				break
			}
			if strings.EqualFold(item.Status, "escalated") || strings.EqualFold(item.Status, "blocked") {
				consecutive++
				continue
			}
		}
		if consecutive >= maxConsecutiveFailures {
			return "consecutive_failures_exhausted", true
		}
	}
	maxRepeatedPrompt := governanceCfg.Budget.MaxRepeatedSamePrompt
	if maxRepeatedPrompt > 0 && promptKey != "" {
		repeated := 0
		for _, item := range traces {
			if item == nil {
				continue
			}
			if strings.TrimSpace(item.ItemID) == strings.TrimSpace(currentItemID) {
				continue
			}
			if strings.TrimSpace(item.PromptFingerprint) == promptKey {
				repeated++
			}
		}
		if repeated >= maxRepeatedPrompt {
			return "repeated_prompt_exhausted", true
		}
	}
	return "", false
}

func (s *Service) loadTracesByRunAndType(ctx context.Context, runID common.RunID, itemType string) ([]*governancemodel.AutoGovernanceTrace, error) {
	if s == nil || s.traces == nil || runID == "" || strings.TrimSpace(itemType) == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.traces.List(ctx, repository.AutoGovernanceTraceQuery{
		RunID:    runID,
		ItemType: itemType,
	})
}

func (s *Service) hasAutoResolvedTrace(ctx context.Context, runID common.RunID, itemType string, itemID string) bool {
	if s == nil || s.traces == nil || runID == "" || itemType == "" || itemID == "" {
		return false
	}
	list, err := s.traces.List(ctx, repository.AutoGovernanceTraceQuery{
		RunID:    runID,
		ItemType: itemType,
		ItemID:   itemID,
		Status:   "resolved",
	})
	if err != nil {
		return false
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Action, "approve") || strings.EqualFold(item.Action, "answer") {
			return true
		}
	}
	return false
}

func (s *Service) hasClosedTrace(ctx context.Context, itemType string, itemID string) bool {
	if s == nil || s.traces == nil || itemType == "" || itemID == "" {
		return false
	}
	list, err := s.traces.List(ctx, repository.AutoGovernanceTraceQuery{
		ItemType: itemType,
		ItemID:   itemID,
	})
	if err != nil {
		return false
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.EqualFold(item.Status, "resolved") || strings.EqualFold(item.Status, "blocked") {
			return true
		}
	}
	return false
}

func classifyApprovalRisk(approval *approvalmodel.Approval) (string, []string) {
	if approval == nil {
		return "L4", []string{"invalid_approval"}
	}
	signals := collectApprovalSignals(approval)
	if containsAnyToken(signals, []string{"secret_access", "production_operation", "external_publish", "destructive_file_change"}) {
		return "L4", signals
	}
	if containsAnyToken(signals, []string{"ssh_write", "external_write", "remote_operation", "network_write", "large_change"}) {
		return "L3", signals
	}
	if containsAnyToken(signals, []string{
		"workspace_write",
		"patch_apply",
		"patch_revert",
		"workspace_dependency_metadata",
		"low_risk_workspace_dependency_metadata",
	}) {
		return "L2", signals
	}
	if containsAnyToken(signals, []string{"read_only", "query", "list", "search"}) {
		return "L1", signals
	}
	return "L3", signals
}

func collectApprovalSignals(approval *approvalmodel.Approval) []string {
	signals := make([]string, 0, 8)
	actionText := normalize(approval.Action + " " + approval.Reason)
	contextText := normalize(flattenApprovalContextText(approval.Context))
	if actionText != "" {
		if containsAnyText(actionText, []string{"read", "list", "query", "search", "fetch", "inspect", "describe", "show"}) {
			signals = append(signals, "read_only")
		}
		if containsAnyText(actionText, []string{"write", "edit", "apply", "patch", "revert", "create", "update"}) {
			signals = append(signals, "workspace_write")
		}
		if containsAnyText(actionText, []string{"delete", "remove", "drop", "truncate", "overwrite"}) {
			signals = append(signals, "destructive_file_change")
		}
		if containsRemoteOperationSignal(actionText, approval.Context) {
			signals = append(signals, "remote_operation")
		}
		if containsAnyText(actionText, []string{"deploy", "publish", "release", "prod", "production"}) {
			signals = append(signals, "production_operation")
		}
	}
	if declaresLowRiskWorkspaceDependencyMetadata(approval, actionText, contextText) {
		signals = append(signals, "workspace_dependency_metadata")
	}
	contextRisk := normalize(stringFromMap(approval.Context, "risk"))
	if contextRisk != "" {
		signals = append(signals, contextRisk)
	}
	contextKind := normalize(stringFromMap(approval.Context, "kind"))
	if contextKind == "patch_approval_required" {
		signals = append(signals, "patch_apply", "workspace_write")
	}
	if contextKind == "patch_revert_approval_required" {
		signals = append(signals, "patch_revert", "workspace_write")
	}
	if containsAnyText(normalize(stringFromMap(approval.Context, "approvalPolicy")), []string{"required_before_apply", "required_before_revert"}) {
		signals = append(signals, "workspace_write")
	}
	if containsRiskReason(approval.Context, "sensitive_path") {
		signals = append(signals, "secret_access")
	}
	if containsRiskReason(approval.Context, "delete_file") || containsRiskReason(approval.Context, "delete_created_file") {
		signals = append(signals, "destructive_file_change")
	}
	if containsRiskReason(approval.Context, "large_change") || containsRiskReason(approval.Context, "lockfile_large_change") {
		signals = append(signals, "large_change")
	}
	if containsAnyText(actionText, []string{"token", "secret", "password", "credential", ".ssh", "key", "pem", "id_rsa", "id_ed25519"}) {
		signals = append(signals, "secret_access")
	}
	return compactUniqueStrings(signals)
}

func declaresLowRiskWorkspaceDependencyMetadata(approval *approvalmodel.Approval, actionText string, contextText string) bool {
	if approval == nil {
		return false
	}
	if actionText == "" && contextText == "" {
		return false
	}
	combined := strings.TrimSpace(actionText + "\n" + contextText)
	if combined == "" {
		return false
	}
	if containsAnyText(combined, []string{"deploy", "publish", "release", "prod", "production", "remote", "ssh", "external", "network"}) {
		return false
	}
	hasWorkspaceWrite := containsAnyText(combined, []string{"write", "edit", "apply", "patch", "create", "update", "tidy", "append"}) ||
		containsAnyText(normalize(stringFromMap(approval.Context, "kind")), []string{"patch_approval_required"})
	if !hasWorkspaceWrite {
		return false
	}
	return containsAnyText(combined, []string{"dependency", "dependencies", "manifest", "metadata", "lockfile", "lock file", "lock", "checksum", "checksums", "module", "modules", "package", "packages", "requirements"})
}

func flattenApprovalContextText(input workorcheventpayload.Payload) string {
	if len(input.Fields) == 0 {
		return ""
	}
	var b strings.Builder
	for _, field := range input.Fields {
		appendApprovalContextValue(&b, field.Value)
	}
	return b.String()
}

func appendApprovalContextValue(b *strings.Builder, value workorcheventpayload.Value) {
	switch value.Kind {
	case workorcheventpayload.ValueString:
		if text := strings.TrimSpace(value.String); text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(text)
		}
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			appendApprovalContextValue(b, item)
		}
	case workorcheventpayload.ValueObject:
		for _, field := range value.Object {
			appendApprovalContextValue(b, field.Value)
		}
	}
}

func containsRemoteOperationSignal(actionText string, contextMap workorcheventpayload.Payload) bool {
	actionText = normalize(actionText)
	if actionText == "" {
		return false
	}
	if containsAnyText(actionText, []string{"ssh", "remote", "远程", "服务器"}) {
		return true
	}
	if containsAnyText(normalize(stringFromMap(contextMap, "risk")), []string{"remote_operation", "ssh_write"}) ||
		containsAnyText(normalize(stringFromMap(contextMap, "kind")), []string{"remote_operation", "ssh_write"}) {
		return true
	}
	for _, phrase := range []string{
		"server host",
		"server endpoint",
		"server operation",
		"server-side operation",
		"server side operation",
	} {
		if strings.Contains(actionText, phrase) {
			return true
		}
	}
	return false
}

func requiresHumanByPolicy(signals []string, requireHumanFor []string) bool {
	if len(signals) == 0 || len(requireHumanFor) == 0 {
		return false
	}
	matched := false
	for _, signal := range signals {
		signal = normalize(signal)
		if signal == "" {
			continue
		}
		for _, required := range requireHumanFor {
			required = normalize(required)
			if required == "" {
				continue
			}
			if signal == required || strings.Contains(signal, required) || strings.Contains(required, signal) {
				matched = true
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func withinDoDBoundary(approval *approvalmodel.Approval, goal *workspace.Goal, dod *workspace.DoD) bool {
	if approval == nil {
		return false
	}
	query := normalize(strings.Join([]string{approval.Reason, approval.Action, stringFromMap(approval.Context, "kind"), stringFromMap(approval.Context, "risk")}, " "))
	if containsAnyText(query, []string{"outside_dod", "out_of_scope", "scope_change", "expand_scope"}) {
		return false
	}
	if containsAnyText(normalize(stringFromMap(approval.Context, "scope")), []string{"outside_dod", "out_of_scope"}) {
		return false
	}
	if containsAnyText(normalize(stringFromMap(approval.Context, "dodStatus")), []string{"mismatch", "outside"}) {
		return false
	}
	_ = goal
	_ = dod
	return true
}

func compactPromptKey(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalize(input)))
	return hex.EncodeToString(sum[:8])
}

func stableContextDigest(input workorcheventpayload.Payload) string {
	if len(input.Fields) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(input.CanonicalString()))
	return hex.EncodeToString(sum[:8])
}

func containsRiskReason(input workorcheventpayload.Payload, target string) bool {
	target = normalize(target)
	if target == "" {
		return false
	}
	reasons, ok := input.StringSlice("riskReasons")
	if !ok {
		return false
	}
	for _, item := range reasons {
		if normalize(item) == target {
			return true
		}
	}
	return false
}

func containsAnyText(input string, terms []string) bool {
	input = normalize(input)
	for _, term := range terms {
		term = normalize(term)
		if term == "" {
			continue
		}
		if strings.Contains(input, term) {
			return true
		}
	}
	return false
}

func containsAnyToken(input []string, terms []string) bool {
	if len(input) == 0 || len(terms) == 0 {
		return false
	}
	set := map[string]struct{}{}
	for _, item := range input {
		item = normalize(item)
		if item != "" {
			set[item] = struct{}{}
		}
	}
	for _, term := range terms {
		term = normalize(term)
		if term == "" {
			continue
		}
		if _, ok := set[term]; ok {
			return true
		}
	}
	return false
}

func tokenSetFromText(input string) []string {
	input = normalize(input)
	if input == "" {
		return nil
	}
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == ' ' || r == ',' || r == '，' || r == ';' || r == '；' || r == ':' || r == '：'
	})
	ret := make([]string, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if len(item) < 2 {
			continue
		}
		ret = append(ret, item)
	}
	return compactUniqueStrings(ret)
}

func compactUniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (s *Service) loadGoalAndDoD(ctx context.Context, goalID common.GoalID, dodID common.DoDID) (*workspace.Goal, *workspace.DoD) {
	var goal *workspace.Goal
	var dod *workspace.DoD
	if goalID != "" {
		item, err := s.workspaceDefs.GetGoal(ctx, goalID)
		if err == nil {
			goal = item
		}
	}
	if dodID != "" {
		item, err := s.workspaceDefs.GetDoD(ctx, dodID)
		if err == nil {
			dod = item
		}
	}
	return goal, dod
}

func (s *Service) loadMemoryItems(ctx context.Context, workspaceID common.WorkspaceID) []*memorymodel.MemoryItem {
	if s == nil || s.memoryItems == nil || workspaceID == "" {
		return nil
	}
	items, err := s.memoryItems.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil
	}
	return items
}

func (s *Service) publishGovernanceEvent(ctx context.Context, topic string, trace *governancemodel.AutoGovernanceTrace, escalationReason string) error {
	if s == nil || s.EventHub() == nil || trace == nil {
		return nil
	}
	cloned := *trace
	cloned.EvidenceRefs = append([]string(nil), trace.EvidenceRefs...)
	event := magicevent.NewEventWithContext(
		topic,
		"kernel/governance",
		governanceevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		governanceevents.GovernanceData{Trace: cloned, EscalationReason: escalationReason},
	)
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: "auto_governance_trace",
		SubjectID:   string(trace.ID),
		WorkspaceID: string(trace.WorkspaceID),
		RunID:       string(trace.RunID),
		StepID:      string(trace.StepID),
	}))
	s.PostEvent(event)
	return nil
}

func isEnabled(value *bool) bool {
	return value != nil && *value
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

package governance

import (
	"context"
	"fmt"
	"strings"
	"time"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

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
		workorcheventpayload.StringField(resolutionPayloadWorkspaceID, string(ticket.WorkspaceID)),
		workorcheventpayload.StringField(resolutionPayloadRunID, string(ticket.RunID)),
		workorcheventpayload.StringField(resolutionPayloadStepID, string(ticket.StepID)),
	).Merge(eval.resolutionPayload)
	semantic := governancesemantics.NormalizeDecisionResolution("", payload)
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
		// A resolved ticket is owner evidence. Runtime continuation is solely
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

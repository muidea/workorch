package governance

import (
	"context"
	"fmt"
	"strings"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

const governanceRecoveryInventoryMaxItems = 100_000

func (s *Service) startGovernanceWorker(ctx context.Context, queue <-chan governanceWork, done chan<- struct{}) error {
	leave, err := s.enterOwner(false)
	if err != nil {
		close(done)
		return err
	}
	run := func() { defer leave(); s.runWorkLoop(ctx, queue, done) }
	if background := s.BackgroundRoutine(); background != nil {
		if err := s.AsyncTask(run); err != nil {
			leave()
			close(done)
			return err
		}
		return nil
	}
	go run()
	return nil
}

func (s *Service) runWorkLoop(ctx context.Context, queue <-chan governanceWork, done chan<- struct{}) {
	defer close(done)
	defer func() {
		// worker panic 时仍保留安全清理路径，不捕获或替换原 panic；
		// 已取消的队列回调只释放自身真实回执。
		s.BeginShutdown()
		s.workEnqueue.Wait()
		for {
			select {
			case work := <-queue:
				if work != nil {
					work(ctx)
				}
			default:
				return
			}
		}
	}()
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
	ctx, queue := s.workCtx, s.workQueue
	if s.closing || s.releasing || ctx == nil || queue == nil {
		s.mu.Unlock()
		return
	}
	leave, err := s.lifecycle.Enter()
	if err != nil {
		s.mu.Unlock()
		return
	}
	s.workEnqueue.Add(1)
	s.mu.Unlock()
	defer s.workEnqueue.Done()
	queued := governanceWork(func(ctx context.Context) {
		defer leave()
		if ctx.Err() == nil {
			work(ctx)
		}
	})
	select {
	case queue <- queued:
	case <-ctx.Done():
		leave()
	case <-time.After(100 * time.Millisecond):
		leave()
	}
}

func (s *Service) scheduleBlockedRunReconciliationLocked() error {
	leave, err := s.enterOwner(false)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.mu.Lock()
	s.reconcileCancel, s.reconcileDone = cancel, done
	closing := s.closing
	s.mu.Unlock()
	if closing {
		cancel()
	}
	reconcile := func() {
		defer close(done)
		defer leave()
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			s.enqueueGovernanceWork(func(workCtx context.Context) {
				if err := s.reconcileIncompleteTraces(workCtx); err != nil {
					return
				}
				_ = s.reconcileBlockedRuns(workCtx)
			})
		case <-ctx.Done():
			return
		}
	}
	if background := s.BackgroundRoutine(); background != nil {
		if err := s.AsyncTask(reconcile); err != nil {
			cancel()
			leave()
			close(done)
			return err
		}
		return nil
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
	_ = s.processPendingTicket(ctx, ticketID)
}

func (s *Service) processPendingTicket(ctx context.Context, ticketID common.TicketID) error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}
	if !isEnabled(cfg.Governance.AutoTicket.Enabled) {
		return nil
	}
	// Ticket IDs are workspace-local. Background and restart workers must
	// receive scope from an immutable owner fact before any cross-owner I/O.
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	closed, err := s.hasClosedTrace(ctx, workspaceID, "ticket", string(ticketID))
	if err != nil {
		return err
	}
	if closed {
		return nil
	}
	ticket, err := s.tickets.Get(ctx, ticketID)
	if err != nil {
		return err
	}
	if ticket == nil || ticket.State != common.DecisionTicketStateOpen {
		return nil
	}
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, ticket.WorkspaceID)
	if err != nil {
		return err
	}
	run, err := s.runs.GetByWorkspaceID(ctx, ticket.WorkspaceID, ticket.RunID)
	if err != nil {
		return err
	}
	if run == nil {
		return common.ErrNotFound
	}
	goal, dod, err := s.loadGoalAndDoD(ctx, run.GoalID, run.DoDID)
	if err != nil {
		return err
	}
	memoryItems, err := s.loadMemoryItems(ctx, ticket.WorkspaceID)
	if err != nil {
		return err
	}
	eval := s.evaluateTicket(ctx, ticket, run, goal, dod, memoryItems)
	if eval.infrastructureErr != nil {
		return eval.infrastructureErr
	}
	trace := &governancemodel.AutoGovernanceTrace{
		ID:                 autoGovernanceTraceID(ticket.WorkspaceID, "ticket", string(ticket.ID)),
		WorkspaceID:        ticket.WorkspaceID,
		RunID:              ticket.RunID,
		StepID:             ticket.StepID,
		ItemType:           "ticket",
		ItemID:             string(ticket.ID),
		Action:             "escalate",
		RiskLevel:          "L1",
		Confidence:         eval.confidence,
		PolicyBlocked:      eval.policyBlocked,
		BudgetBefore:       eval.budgetBefore,
		BudgetAfter:        eval.budgetAfter,
		Reason:             eval.reason,
		EvidenceRefs:       eval.evidenceRefs,
		PromptFingerprint:  eval.promptKey,
		LLMDecisionPreview: eval.promptKey,
		OccurredAt:         nowUTC(),
	}
	trace, err = s.persistEvaluatedGovernanceTrace(ctx, trace)
	if err != nil {
		return err
	}
	if err := s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEvaluated, trace, ""); err != nil {
		return err
	}

	if !eval.canAutoResolve {
		trace, err = s.escalateGovernanceTrace(ctx, trace, eval.policyBlocked)
		if err != nil {
			return err
		}
		return s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEscalated, trace, eval.policyBlocked)
	}
	payload := workorcheventpayload.New(
		workorcheventpayload.StringField("answer", eval.answer),
		workorcheventpayload.StringField("source", autoGovernanceActor),
		workorcheventpayload.StringField("resolvedBy", autoGovernanceActor),
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

	// Persist the exact intended owner mutation before sending it. A restart
	// can now distinguish an interrupted ensure from a mere evaluation.
	trace, err = s.startGovernanceTraceResolution(ctx, trace, "answer", eval.policyMatched, eval.budgetAfter, eval.reason)
	if err != nil {
		return err
	}
	if err := s.resolve(ctx, ticket.ID, semantic.ResolutionType, payload); err != nil {
		trace, traceErr := s.blockGovernanceTrace(ctx, trace, "resolve_failed", err.Error())
		if traceErr != nil {
			return traceErr
		}
		return s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoBlocked, trace, "resolve_failed")
	}
	trace, err = s.resolveGovernanceTrace(ctx, trace, "answer", "")
	if err != nil {
		return err
	}
	return s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoResolved, trace, "")
}

func (s *Service) processPendingApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}
	if !isEnabled(cfg.Governance.AutoApproval.Enabled) {
		return nil
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" || runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	closed, err := s.hasClosedApprovalTrace(ctx, workspaceID, runID, approvalID)
	if err != nil {
		return err
	}
	if closed {
		return nil
	}
	approval, err := s.approvals.GetScoped(ctx, workspaceID, runID, approvalID)
	if err != nil {
		return err
	}
	if approval == nil || approval.State != common.ApprovalStatePending {
		return nil
	}
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, approval.WorkspaceID)
	if err != nil {
		return err
	}
	run, err := s.runs.GetByWorkspaceID(ctx, approval.WorkspaceID, approval.RunID)
	if err != nil {
		return err
	}
	if run == nil {
		return common.ErrNotFound
	}
	goal, dod, err := s.loadGoalAndDoD(ctx, run.GoalID, run.DoDID)
	if err != nil {
		return err
	}
	eval := s.evaluateApproval(ctx, approval, goal, dod, cfg.Governance)
	if eval.infrastructureErr != nil {
		return eval.infrastructureErr
	}
	trace := &governancemodel.AutoGovernanceTrace{
		ID:                 autoGovernanceApprovalTraceID(approval.WorkspaceID, approval.RunID, approval.ID),
		WorkspaceID:        approval.WorkspaceID,
		RunID:              approval.RunID,
		StepID:             approval.StepID,
		ItemType:           "approval",
		ItemID:             string(approval.ID),
		Action:             "escalate",
		RiskLevel:          eval.riskLevel,
		Confidence:         eval.confidence,
		PolicyBlocked:      eval.policyBlocked,
		BudgetBefore:       eval.budgetBefore,
		BudgetAfter:        eval.budgetAfter,
		Reason:             eval.reason,
		EvidenceRefs:       eval.evidenceRefs,
		PromptFingerprint:  eval.promptKey,
		LLMDecisionPreview: eval.promptKey,
		OccurredAt:         nowUTC(),
	}
	trace, err = s.persistEvaluatedGovernanceTrace(ctx, trace)
	if err != nil {
		return err
	}
	if err := s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEvaluated, trace, ""); err != nil {
		return err
	}

	if !eval.canAutoApprove {
		trace, err = s.escalateGovernanceTrace(ctx, trace, eval.policyBlocked)
		if err != nil {
			return err
		}
		return s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoEscalated, trace, eval.policyBlocked)
	}

	trace, err = s.startGovernanceTraceResolution(ctx, trace, "approve", eval.policyMatched, eval.budgetAfter, eval.reason)
	if err != nil {
		return err
	}
	if err := s.resolveApproval(ctx, approval.ID, approval.RunID); err != nil {
		current, readErr := s.approvals.GetScoped(ctx, approval.WorkspaceID, approval.RunID, approval.ID)
		if readErr == nil && approvalResolutionRecoveryPending(current) {
			// Approval has durably accepted the trusted proposal or terminal state,
			// but its SM-2/evidence delivery has not completed. Keep the trace
			// resolving so both owners can recover; blocked is a closed budget state.
			return err
		}
		if readErr == nil && approvalResolutionDeliveredByGovernance(current) {
			return s.finalizeRecoveredTrace(ctx, trace, "approve")
		}
		trace, traceErr := s.blockGovernanceTrace(ctx, trace, "resolve_failed", err.Error())
		if traceErr != nil {
			return traceErr
		}
		return s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoBlocked, trace, "resolve_failed")
	}

	trace, err = s.resolveGovernanceTrace(ctx, trace, "approve", "")
	if err != nil {
		return err
	}
	return s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoResolved, trace, "")
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
			_ = s.processPendingTicket(ctx, ticket.ID)
			return
		}
		if ticket.State == common.DecisionTicketStateResolved {
			_ = s.reconcileIncompleteItemTrace(ctx, ticket.WorkspaceID, ticket.RunID, "ticket", string(ticket.ID))
		}
		// Runtime continuation remains solely owned by Orchestrator.
		return
	}
	if data.State == common.RunStateBlockedOnApproval && data.BlockingApprovalID != "" {
		approval, err := s.approvals.GetScoped(ctx, data.WorkspaceID, data.RunID, data.BlockingApprovalID)
		if err != nil || approval == nil {
			return
		}
		if approval.State == common.ApprovalStatePending {
			_ = s.processPendingApproval(ctx, approval.RunID, approval.ID)
			return
		}
		if approval.State == common.ApprovalStateApproved {
			_ = s.reconcileIncompleteItemTrace(ctx, approval.WorkspaceID, approval.RunID, "approval", string(approval.ID))
		}
		// Governance never drives a Run.
	}
}

func (s *Service) reconcileIncompleteTraces(ctx context.Context) error {
	if s == nil || s.traces == nil {
		return common.ErrInvalidArgument
	}
	items, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{Limit: governanceRecoveryInventoryMaxItems + 1})
	if err != nil {
		return err
	}
	if len(items) > governanceRecoveryInventoryMaxItems {
		return fmt.Errorf("%w: governance trace recovery inventory exceeds %d items", common.ErrConflict, governanceRecoveryInventoryMaxItems)
	}
	latest := map[string]*governancemodel.AutoGovernanceTrace{}
	for _, item := range items {
		if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || strings.TrimSpace(item.ItemType) == "" || strings.TrimSpace(item.ItemID) == "" {
			return fmt.Errorf("%w: governance trace recovery inventory contains an invalid owner fact", common.ErrConflict)
		}
		if !isIncompleteGovernanceTrace(item) {
			continue
		}
		key := string(item.WorkspaceID) + "\x00" + string(item.RunID) + "\x00" + strings.ToLower(strings.TrimSpace(item.ItemType)) + "\x00" + strings.TrimSpace(item.ItemID)
		prior := latest[key]
		if prior == nil || item.OccurredAt >= prior.OccurredAt {
			latest[key] = item
		}
	}
	var firstErr error
	for _, item := range latest {
		if err := s.reconcileIncompleteTrace(ctx, item); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) reconcileIncompleteItemTrace(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, itemType string, itemID string) error {
	if s == nil || s.traces == nil || workspaceID == "" || runID == "" || strings.TrimSpace(itemType) == "" || strings.TrimSpace(itemID) == "" {
		return common.ErrInvalidArgument
	}
	items, err := s.traces.List(ctx, workorchquerycontract.AutoGovernanceTraceQuery{
		WorkspaceID: workspaceID,
		RunID:       runID,
		ItemType:    itemType,
		ItemID:      itemID,
	})
	if err != nil {
		return err
	}
	var latest *governancemodel.AutoGovernanceTrace
	for _, item := range items {
		if !isIncompleteGovernanceTrace(item) {
			continue
		}
		if latest == nil || item.OccurredAt >= latest.OccurredAt {
			latest = item
		}
	}
	if latest == nil {
		return nil
	}
	return s.reconcileIncompleteTrace(ctx, latest)
}

func isIncompleteGovernanceTrace(item *governancemodel.AutoGovernanceTrace) bool {
	if item == nil || item.WorkspaceID == "" || strings.TrimSpace(item.ItemType) == "" || strings.TrimSpace(item.ItemID) == "" {
		return false
	}
	return autoGovernanceTraceStateIs(item.Status, governancemodel.AutoGovernanceTraceStateEvaluated) || autoGovernanceTraceStateIs(item.Status, governancemodel.AutoGovernanceTraceStateResolving)
}

func (s *Service) reconcileIncompleteTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace) error {
	if !isIncompleteGovernanceTrace(trace) {
		return nil
	}
	workspaceCtx, err := common.BindRuntimeWorkspaceContext(ctx, trace.WorkspaceID)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(trace.ItemType)) {
	case "ticket":
		item, err := s.tickets.Get(workspaceCtx, common.TicketID(trace.ItemID))
		if err != nil {
			return err
		}
		return s.reconcileTicketTrace(workspaceCtx, trace, item)
	case "approval":
		item, err := s.approvals.GetScoped(workspaceCtx, trace.WorkspaceID, trace.RunID, common.ApprovalID(trace.ItemID))
		if err != nil {
			return err
		}
		return s.reconcileApprovalTrace(workspaceCtx, trace, item)
	default:
		return s.blockIncompleteTrace(workspaceCtx, trace, "unsupported_item_type")
	}
}

func (s *Service) reconcileTicketTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace, ticket *decisionmodel.Ticket) error {
	if ticket == nil || ticket.WorkspaceID != trace.WorkspaceID || ticket.RunID != trace.RunID || string(ticket.ID) != trace.ItemID {
		return s.blockIncompleteTrace(ctx, trace, "owner_scope_mismatch")
	}
	switch ticket.State {
	case common.DecisionTicketStateOpen:
		return s.processPendingTicket(ctx, ticket.ID)
	case common.DecisionTicketStateResolved:
		if autoGovernanceTraceStateIs(trace.Status, governancemodel.AutoGovernanceTraceStateResolving) || strings.EqualFold(ticket.ResolvedBy, autoGovernanceActor) {
			return s.finalizeRecoveredTrace(ctx, trace, "answer")
		}
		return s.blockIncompleteTrace(ctx, trace, "owner_resolved_externally")
	default:
		return s.blockIncompleteTrace(ctx, trace, "owner_state_changed")
	}
}

func (s *Service) reconcileApprovalTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace, approval *approvalmodel.Approval) error {
	if approval == nil || approval.WorkspaceID != trace.WorkspaceID || approval.RunID != trace.RunID || string(approval.ID) != trace.ItemID {
		return s.blockIncompleteTrace(ctx, trace, "owner_scope_mismatch")
	}
	switch approval.State {
	case common.ApprovalStatePending:
		return s.processPendingApproval(ctx, approval.RunID, approval.ID)
	case common.ApprovalStateApproved:
		if approvalResolutionRecoveryPending(approval) {
			return nil
		}
		if autoGovernanceTraceStateIs(trace.Status, governancemodel.AutoGovernanceTraceStateResolving) || approvalResolutionDeliveredByGovernance(approval) {
			return s.finalizeRecoveredTrace(ctx, trace, "approve")
		}
		return s.blockIncompleteTrace(ctx, trace, "owner_resolved_externally")
	default:
		return s.blockIncompleteTrace(ctx, trace, "owner_state_changed")
	}
}

func approvalResolvedByGovernance(item *approvalmodel.Approval) bool {
	if item == nil {
		return false
	}
	principal := item.ResolutionPrincipal.Normalized()
	return principal.Kind == approvalmodel.ApprovalResolutionPrincipalAutoGovernance && principal.SubjectID == governancecommon.ModuleID
}

func approvalResolutionRecoveryPending(item *approvalmodel.Approval) bool {
	if item == nil {
		return false
	}
	if item.State == common.ApprovalStatePending && item.ResolutionProposal != nil {
		proposal := item.ResolutionProposal.Clone()
		return proposal.State == approvalmodel.ResolutionProposalPending &&
			proposal.Principal.Kind == approvalmodel.ApprovalResolutionPrincipalAutoGovernance && proposal.Principal.SubjectID == governancecommon.ModuleID
	}
	return item.State == common.ApprovalStateApproved && approvalResolvedByGovernance(item) &&
		(item.EvidenceRecordedAt == "" || item.ResolutionNotificationPublishedAt == "")
}

func approvalResolutionDeliveredByGovernance(item *approvalmodel.Approval) bool {
	return item != nil && item.State == common.ApprovalStateApproved && approvalResolvedByGovernance(item) &&
		item.EvidenceRecordedAt != "" && item.ResolutionNotificationPublishedAt != ""
}

func (s *Service) finalizeRecoveredTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace, action string) error {
	var err error
	trace, err = s.resolveGovernanceTrace(ctx, trace, action, "recovered committed auto-governance owner state")
	if err != nil {
		return err
	}
	return s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoResolved, trace, "")
}

func (s *Service) blockIncompleteTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace, reason string) error {
	var err error
	trace, err = s.blockGovernanceTrace(ctx, trace, reason, reason)
	if err != nil {
		return err
	}
	return s.publishGovernanceEvent(ctx, governanceevents.TopicGovernanceAutoBlocked, trace, reason)
}

func (s *Service) reconcileBlockedRuns(ctx context.Context) error {
	if s == nil || s.runs == nil {
		return common.ErrInvalidArgument
	}
	runs, err := s.runs.List(ctx)
	if err != nil {
		return err
	}
	if len(runs) > governanceRecoveryInventoryMaxItems {
		return fmt.Errorf("%w: Run recovery inventory exceeds %d items", common.ErrConflict, governanceRecoveryInventoryMaxItems)
	}
	for _, item := range runs {
		if item == nil || item.WorkspaceID == "" {
			return fmt.Errorf("%w: Run recovery inventory contains an invalid owner fact", common.ErrConflict)
		}
		workspaceCtx, scopeErr := common.BindRuntimeWorkspaceContext(ctx, item.WorkspaceID)
		if scopeErr != nil {
			continue
		}
		switch item.State {
		case common.RunStateBlockedOnDecision:
			if item.BlockingTicketID != "" {
				_ = s.processPendingTicket(workspaceCtx, item.BlockingTicketID)
			}
		case common.RunStateBlockedOnApproval:
			if item.BlockingApprovalID != "" {
				_ = s.processPendingApproval(workspaceCtx, item.ID, item.BlockingApprovalID)
			}
		}
	}
	return nil
}

package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

var structuredValuePattern = regexp.MustCompile(`([A-Za-z0-9_\-\p{Han}]{2,32})\s*[:：=]\s*([^\s,，。;；]+)`)

const (
	recoveryPolicyRuntimeRepairExhausted = "runtime_repair_exhausted_requires_strategy"
)

type ResolveDecisionFunc func(context.Context, common.TicketID, string, map[string]any) error
type ResolveApprovalFunc func(context.Context, common.ApprovalID) error
type ResumeRunFunc func(context.Context, common.RunID) error

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

type Service struct {
	tickets         repository.DecisionTicketRepository
	resolutions     repository.DecisionResolutionRepository
	approvals       repository.ApprovalRepository
	runs            repository.RunRepository
	goals           repository.GoalRepository
	dods            repository.DoDRepository
	memoryItems     repository.MemoryItemRepository
	traces          repository.AutoGovernanceTraceRepository
	eventHub        events.Hub
	resolve         ResolveDecisionFunc
	resolveApproval ResolveApprovalFunc
	resume          ResumeRunFunc
	loadConfig      func() (workorchconfig.Config, error)
	mu              sync.Mutex
	unsubscribes    []func()
}

func NewService(deps Dependencies) *Service {
	return &Service{
		tickets:         deps.Tickets,
		resolutions:     deps.Resolutions,
		approvals:       deps.Approvals,
		runs:            deps.Runs,
		goals:           deps.Goals,
		dods:            deps.DoDs,
		memoryItems:     deps.MemoryItems,
		traces:          deps.AutoGovernanceTraces,
		eventHub:        deps.EventHub,
		resolve:         deps.ResolveDecision,
		resolveApproval: deps.ResolveApproval,
		resume:          deps.ResumeRun,
		loadConfig:      deps.LoadConfig,
	}
}

func (s *Service) Start() error {
	if s == nil || s.eventHub == nil || s.tickets == nil || s.approvals == nil || s.runs == nil || s.goals == nil || s.dods == nil || s.memoryItems == nil || s.traces == nil || s.resolve == nil || s.resolveApproval == nil || s.resume == nil {
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
	unsubTicket := s.eventHub.Subscribe(events.TopicTicketCreated, func(ctx context.Context, envelope events.Envelope) error {
		s.processTicketCreated(ctx, envelope)
		return nil
	})
	unsubApproval := s.eventHub.Subscribe(events.TopicApprovalCreated, func(ctx context.Context, envelope events.Envelope) error {
		s.processApprovalCreated(ctx, envelope)
		return nil
	})
	unsubRunState := s.eventHub.Subscribe(events.TopicRunStateChanged, func(ctx context.Context, envelope events.Envelope) error {
		s.processRunStateChanged(ctx, envelope)
		return nil
	})
	s.unsubscribes = []func(){unsubTicket, unsubApproval, unsubRunState}
	s.reconcileBlockedRuns(context.Background())
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, unsub := range s.unsubscribes {
		if unsub != nil {
			unsub()
		}
	}
	s.unsubscribes = nil
}

func (s *Service) processTicketCreated(ctx context.Context, envelope events.Envelope) {
	cfg, err := s.loadConfig()
	if err != nil || !isEnabled(cfg.Governance.AutoTicket.Enabled) {
		return
	}
	ticketID := common.TicketID(strings.TrimSpace(envelope.SubjectID))
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
	if ticketID == "" || s.hasClosedTrace(ctx, "ticket", string(ticketID)) {
		return
	}
	ticket, err := s.tickets.Get(ctx, ticketID)
	if err != nil || ticket == nil || ticket.State != common.DecisionTicketStateOpen {
		return
	}
	run, err := s.runs.Get(ctx, ticket.RunID)
	if err != nil || run == nil {
		return
	}
	goal, dod := s.loadGoalAndDoD(ctx, run.GoalID, run.DoDID)
	memoryItems := s.loadMemoryItems(ctx, ticket.WorkspaceID)
	eval := s.evaluateTicket(ctx, ticket, goal, dod, memoryItems)
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
	_ = s.publishGovernanceEvent(ctx, events.TopicGovernanceAutoEvaluated, trace, "")

	if !eval.canAutoResolve {
		trace.Status = "escalated"
		trace.Action = "escalate"
		trace.OccurredAt = nowUTC()
		_ = s.traces.Upsert(ctx, trace)
		_ = s.publishGovernanceEvent(ctx, events.TopicGovernanceAutoEscalated, trace, eval.policyBlocked)
		return
	}
	payload := map[string]any{
		"answer":     eval.answer,
		"source":     "auto_governance",
		"confidence": eval.confidence,
		"reason":     eval.reason,
	}
	for key, value := range eval.resolutionPayload {
		payload[key] = value
	}
	semantic := governancecommon.NormalizeDecisionResolution("", payload)
	payload["governanceSemantic"] = map[string]any{
		"resolutionType": semantic.ResolutionType,
		"intent":         semantic.Intent,
		"confidence":     semantic.Confidence,
		"rawInput":       semantic.RawInput,
		"source":         semantic.Source,
		"inferred":       semantic.Inferred,
		"version":        "v1",
	}
	if err := s.resolve(ctx, ticket.ID, semantic.ResolutionType, payload); err != nil {
		trace.Status = "blocked"
		trace.Action = "escalate"
		trace.PolicyBlocked = "resolve_failed"
		trace.Reason = err.Error()
		trace.OccurredAt = nowUTC()
		_ = s.traces.Upsert(ctx, trace)
		_ = s.publishGovernanceEvent(ctx, events.TopicGovernanceAutoBlocked, trace, "resolve_failed")
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
	_ = s.publishGovernanceEvent(ctx, events.TopicGovernanceAutoResolved, trace, "")

	_ = s.resume(ctx, ticket.RunID)
}

func (s *Service) processApprovalCreated(ctx context.Context, envelope events.Envelope) {
	cfg, err := s.loadConfig()
	if err != nil || !isEnabled(cfg.Governance.AutoApproval.Enabled) {
		return
	}
	approvalID := common.ApprovalID(strings.TrimSpace(envelope.SubjectID))
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
	if approvalID == "" || s.hasClosedTrace(ctx, "approval", string(approvalID)) {
		return
	}
	approval, err := s.approvals.Get(ctx, approvalID)
	if err != nil || approval == nil || approval.State != common.ApprovalStatePending {
		return
	}
	run, err := s.runs.Get(ctx, approval.RunID)
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
	_ = s.publishGovernanceEvent(ctx, events.TopicGovernanceAutoEvaluated, trace, "")

	if !eval.canAutoApprove {
		trace.Status = "escalated"
		trace.Action = "escalate"
		trace.OccurredAt = nowUTC()
		_ = s.traces.Upsert(ctx, trace)
		_ = s.publishGovernanceEvent(ctx, events.TopicGovernanceAutoEscalated, trace, eval.policyBlocked)
		return
	}

	if err := s.resolveApproval(ctx, approval.ID); err != nil {
		trace.Status = "blocked"
		trace.Action = "escalate"
		trace.PolicyBlocked = "resolve_failed"
		trace.Reason = err.Error()
		trace.OccurredAt = nowUTC()
		_ = s.traces.Upsert(ctx, trace)
		_ = s.publishGovernanceEvent(ctx, events.TopicGovernanceAutoBlocked, trace, "resolve_failed")
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
	_ = s.publishGovernanceEvent(ctx, events.TopicGovernanceAutoResolved, trace, "")

	_ = s.resume(ctx, approval.RunID)
}

func (s *Service) processRunStateChanged(ctx context.Context, envelope events.Envelope) {
	data, ok := events.DecodeRunPayload(envelope.Payload)
	if !ok {
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
		resolution, err := s.resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil || resolution == nil {
			return
		}
		source := strings.TrimSpace(stringFromMap(resolution.Payload, "source"))
		if !strings.EqualFold(source, "auto_governance") {
			return
		}
		_ = s.resume(ctx, ticket.RunID)
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
		if !s.hasAutoResolvedTrace(ctx, approval.RunID, "approval", string(approval.ID)) {
			return
		}
		_ = s.resume(ctx, approval.RunID)
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
		if item == nil {
			continue
		}
		switch item.State {
		case common.RunStateBlockedOnDecision:
			if item.BlockingTicketID != "" {
				s.processPendingTicket(ctx, item.BlockingTicketID)
			}
		case common.RunStateBlockedOnApproval:
			if item.BlockingApprovalID != "" {
				s.processPendingApproval(ctx, item.BlockingApprovalID)
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
	resolutionPayload map[string]any
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

func (s *Service) evaluateTicket(ctx context.Context, ticket *decisionmodel.Ticket, goal *workspace.Goal, dod *workspace.DoD, memoryItems []*memorymodel.MemoryItem) ticketEvaluation {
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
	if handled, recoveryEval := evaluateRuntimeRepairExhaustionTicket(ticket, ret); handled {
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
		ret.resolutionPayload = map[string]any{
			"recoveryAction": "replan",
		}
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

func evaluateRuntimeRepairExhaustionTicket(ticket *decisionmodel.Ticket, base ticketEvaluation) (bool, ticketEvaluation) {
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
	ret.canAutoResolve = true
	ret.answer = "replan"
	ret.confidence = 0.91
	ret.reason = "runtime repair exhausted and structured recovery policy allows replan"
	ret.policyBlocked = ""
	ret.policyMatched = "runtime_repair_exhausted_auto_replan"
	ret.resolutionPayload = map[string]any{
		"recoveryAction": "replan",
	}
	return true, ret
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
	ctxMap map[string]any,
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

func hasCandidateValues(input map[string]any) bool {
	if input == nil {
		return false
	}
	raw, ok := input["candidateValues"]
	return ok && raw != nil
}

func continuationCandidateAnswer(input map[string]any) (string, float64, bool) {
	if input == nil {
		return "", 0, false
	}
	raw, ok := input["candidateValues"]
	if !ok || raw == nil {
		return "", 0, false
	}
	candidates := map[string]any{}
	switch typed := raw.(type) {
	case map[string]any:
		candidates = typed
	case map[string]string:
		for key, value := range typed {
			candidates[key] = value
		}
	default:
		return "", 0, false
	}
	if len(candidates) == 0 {
		return "", 0, false
	}
	confidences := confidenceMapFromMap(input, "confidenceByField")
	selected := map[string]string{}
	confidence := 1.0
	confidenceSeen := false
	for field, value := range candidates {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		candidate := preferredAutoContinuationCandidate(value)
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

func preferredAutoContinuationCandidate(value any) string {
	switch typed := value.(type) {
	case string:
		if isAutoContinuationCandidateText(typed) {
			return strings.TrimSpace(typed)
		}
	case []string:
		for _, item := range typed {
			if isAutoContinuationCandidateText(item) {
				return strings.TrimSpace(item)
			}
		}
	case []any:
		for _, item := range typed {
			if candidate := preferredAutoContinuationCandidate(item); candidate != "" {
				return candidate
			}
		}
	case map[string]any:
		for _, key := range []string{"value", "id", "mode", "target", "name"} {
			if candidate := preferredAutoContinuationCandidate(typed[key]); candidate != "" {
				return candidate
			}
		}
		for _, nested := range typed {
			if candidate := preferredAutoContinuationCandidate(nested); candidate != "" {
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

func structuredCandidateAnswer(input map[string]any) (string, float64, bool) {
	if input == nil {
		return "", 0, false
	}
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
	topConfidence, hasTopConfidence := confidenceValue(input["confidence"])

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

func hasManualRequiredMarker(input map[string]any) bool {
	if input == nil {
		return false
	}
	return boolFromMap(input, "humanRequired") ||
		boolFromMap(input, "requiresHuman") ||
		boolFromMap(input, "manualRequired") ||
		len(stringSliceFromMap(input, "requiresHumanFields")) > 0
}

func runtimeRecoveryRequiresExternalOperator(ctxMap map[string]any, question string, reason string) bool {
	if ctxMap == nil {
		return false
	}
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

func firstNonEmptyMapValue(input map[string]any, keys []string) string {
	for _, key := range keys {
		value := strings.TrimSpace(stringFromMap(input, key))
		if value != "" {
			return value
		}
	}
	return ""
}

func singleStringMapValue(input map[string]any) string {
	if input == nil {
		return ""
	}
	if len(input) != 1 {
		return ""
	}
	value := ""
	for _, raw := range input {
		text, ok := raw.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if value != "" {
			return ""
		}
		value = text
	}
	return value
}

func stringMapFromMap(input map[string]any, key string) map[string]string {
	ret := map[string]string{}
	raw, ok := input[key]
	if !ok || raw == nil {
		return ret
	}
	switch typed := raw.(type) {
	case map[string]any:
		for k, v := range typed {
			if value := stringifyScalar(v); value != "" {
				ret[strings.TrimSpace(k)] = value
			}
		}
	case map[string]string:
		for k, v := range typed {
			if value := strings.TrimSpace(v); value != "" {
				ret[strings.TrimSpace(k)] = value
			}
		}
	}
	return ret
}

func confidenceMapFromMap(input map[string]any, key string) map[string]float64 {
	ret := map[string]float64{}
	raw, ok := input[key]
	if !ok || raw == nil {
		return ret
	}
	switch typed := raw.(type) {
	case map[string]any:
		for k, v := range typed {
			if value, ok := confidenceValue(v); ok {
				ret[normalize(k)] = value
			}
		}
	case map[string]float64:
		for k, v := range typed {
			ret[normalize(k)] = v
		}
	}
	return ret
}

func stringSliceFromMap(input map[string]any, key string) []string {
	raw, ok := input[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		ret := make([]string, 0, len(typed))
		for _, value := range typed {
			if value = strings.TrimSpace(value); value != "" {
				ret = append(ret, value)
			}
		}
		return ret
	case []any:
		ret := make([]string, 0, len(typed))
		for _, value := range typed {
			if text := stringifyScalar(value); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	}
	return nil
}

func boolFromMap(input map[string]any, key string) bool {
	raw, ok := input[key]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	}
	return false
}

func confidenceValue(input any) (float64, bool) {
	switch typed := input.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		value, err := typed.Float64()
		return value, err == nil
	case string:
		switch normalize(typed) {
		case "high":
			return 0.9, true
		case "medium":
			return 0.75, true
		case "low":
			return 0.5, true
		default:
			value, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			return value, err == nil
		}
	}
	return 0, false
}

func stringifyScalar(input any) string {
	switch typed := input.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, bool, json.Number:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
	return ""
}

func minFloat(a float64, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func stringFromMap(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, ok := input[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
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
	if containsAnyToken(signals, []string{"workspace_write", "patch_apply", "patch_revert"}) {
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

func containsRemoteOperationSignal(actionText string, contextMap map[string]any) bool {
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

func stableContextDigest(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	bytes, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:8])
}

func containsRiskReason(input map[string]any, target string) bool {
	target = normalize(target)
	if target == "" {
		return false
	}
	switch reasons := input["riskReasons"].(type) {
	case []any:
		for _, item := range reasons {
			text, ok := item.(string)
			if !ok {
				continue
			}
			if normalize(text) == target {
				return true
			}
		}
	case []string:
		for _, item := range reasons {
			if normalize(item) == target {
				return true
			}
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
		item, err := s.goals.Get(ctx, goalID)
		if err == nil {
			goal = item
		}
	}
	if dodID != "" {
		item, err := s.dods.Get(ctx, dodID)
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
	if s == nil || s.eventHub == nil || trace == nil {
		return nil
	}
	return s.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: "kernel/governance",
		SubjectType:  "auto_governance_trace",
		SubjectID:    string(trace.ID),
		WorkspaceID:  string(trace.WorkspaceID),
		RunID:        string(trace.RunID),
		StepID:       string(trace.StepID),
		Payload: events.AutoGovernancePayload(
			trace.ID,
			trace.ItemType,
			trace.ItemID,
			trace.Action,
			trace.RiskLevel,
			trace.Confidence,
			trace.PolicyMatched,
			trace.PolicyBlocked,
			trace.BudgetBefore,
			trace.BudgetAfter,
			trace.LLMProviderID,
			trace.LLMModel,
			trace.LLMDecisionPreview,
			trace.PromptFingerprint,
			trace.Status,
			trace.Reason,
			escalationReason,
		),
	})
}

func isEnabled(value *bool) bool {
	return value != nil && *value
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

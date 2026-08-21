package governance

import (
	"context"
	"strings"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	recoveryPolicyRuntimeRepairExhausted = "runtime_repair_exhausted_requires_strategy"
)

type ticketEvaluation struct {
	canAutoResolve    bool
	infrastructureErr error
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
	if ticket.DefinitionClarification != nil || ticket.DecisionKind == "workspace_definition_clarification" || ticket.DecisionKind == "runtime_definition_clarification" {
		ret.policyBlocked = "definition_clarification_requires_owner_contract"
		ret.reason = "Definition clarification can only be resolved by corrected admission or a Run-owned replacement intent"
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
	budgetUsed, err := s.countResolvedTicketTraces(ctx, ticket.WorkspaceID, ticket.RunID)
	if err != nil {
		ret.infrastructureErr = err
		ret.policyBlocked = "trace_query_failed"
		ret.reason = "governance trace budget is unavailable"
		return ret
	}
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
	blockedReason, blocked, err := s.blockedBySharedBudgetPolicy(ctx, cfg.Governance, ticket.WorkspaceID, ticket.RunID, "ticket", string(ticket.ID), ret.promptKey)
	if err != nil {
		ret.infrastructureErr = err
		ret.policyBlocked = "trace_query_failed"
		ret.reason = "governance trace policy history is unavailable"
		return ret
	}
	if blocked {
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
	ret.policyBlocked = "explicit_recovery_strategy_required"
	ret.reason = "runtime repair exhaustion requires an explicit recovery strategy"
	return true, ret
}

package governancecore

import (
	"strings"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
)

func IsRuntimeRepairExhaustionTicket(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	return strings.TrimSpace(StringFromDecisionPayload(ticket.Context, decisionContextKeyRecoveryPolicy)) == decisionContextRecoveryPolicyRepairPlan
}

func IsAmbiguousRuntimeRepairExhaustionResolution(payload map[string]any) bool {
	if len(payload) == 0 {
		return true
	}
	if action := InferRuntimeRepairRecoveryAction(payload); action != "" {
		switch action {
		case "retry_same_step", "replan", "provide_information", "terminate":
			return false
		default:
			return true
		}
	}
	return true
}

func InferRuntimeRepairRecoveryAction(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	if rawAction := strings.TrimSpace(StringFromDecisionPayload(payload, "recoveryAction")); rawAction != "" {
		return NormalizeRuntimeRepairRecoveryAction(rawAction)
	}
	semantic := DecisionPayloadMap(payload["governanceSemantic"])
	intent := strings.TrimSpace(StringFromDecisionPayload(semantic, "intent"))
	switch intent {
	case governancecommon.GovernanceIntentRequestReplan:
		return "replan"
	case governancecommon.GovernanceIntentRejectExecution, governancecommon.GovernanceIntentRequestTerminate:
		return "terminate"
	case governancecommon.GovernanceIntentApproveExecution:
		return "retry_same_step"
	}
	text := SemanticTextForRepairResolution(payload)
	if action := NormalizeRuntimeRepairRecoveryAction(text); action != "" {
		return action
	}
	if intent == governancecommon.GovernanceIntentProvideMissingInfo && strings.TrimSpace(text) != "" {
		return "provide_information"
	}
	if strings.TrimSpace(text) != "" {
		return "provide_information"
	}
	return ""
}

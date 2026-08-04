package governancecore

import (
	"strings"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func IsRuntimeRepairExhaustionTicket(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	return strings.TrimSpace(StringFromDecisionPayload(ticket.Context, decisionContextKeyRecoveryPolicy)) == decisionContextRecoveryPolicyRepairPlan
}

func IsAmbiguousRuntimeRepairExhaustionResolution(payload workorcheventpayload.Payload) bool {
	if len(payload.Fields) == 0 {
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

func InferRuntimeRepairRecoveryAction(payload workorcheventpayload.Payload) string {
	if len(payload.Fields) == 0 {
		return ""
	}
	if rawAction := strings.TrimSpace(StringFromDecisionPayload(payload, "recoveryAction")); rawAction != "" {
		return NormalizeRuntimeRepairRecoveryAction(rawAction)
	}
	semantic := DecisionPayloadObject(payload, "governanceSemantic")
	intent := strings.TrimSpace(StringFromDecisionPayload(semantic, "intent"))
	switch intent {
	case governancesemantics.IntentRequestReplan:
		return "replan"
	case governancesemantics.IntentRejectExecution, governancesemantics.IntentRequestTerminate:
		return "terminate"
	case governancesemantics.IntentApproveExecution:
		return "retry_same_step"
	}
	text := SemanticTextForRepairResolution(payload)
	if action := NormalizeRuntimeRepairRecoveryAction(text); action != "" {
		return action
	}
	if intent == governancesemantics.IntentProvideMissingInfo && strings.TrimSpace(text) != "" {
		return "provide_information"
	}
	if strings.TrimSpace(text) != "" {
		return "provide_information"
	}
	return ""
}

package governancecore

import (
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
)

func NormalizeDecisionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload map[string]any, resolutionType string) (map[string]any, string) {
	payload = NormalizeCandidateSelectionResolutionPayloadForTicket(ticket, payload)
	if IsWorkorchRuntimeLLMProviderRecoveryTicket(ticket) {
		action := InferRuntimeRepairRecoveryAction(payload)
		if action == "retry_same_step" {
			ret := CloneDecisionPayload(payload)
			if ret == nil {
				ret = map[string]any{}
			}
			ret["recoveryAction"] = action
			return ret, decisioncommon.ResolutionTypeProvideMissingInfo
		}
	}
	if !IsRuntimeRepairExhaustionTicket(ticket) {
		return payload, resolutionType
	}
	action := InferRuntimeRepairRecoveryAction(payload)
	if action != "" {
		action = NormalizeRuntimeRepairRecoveryActionByTicketPolicy(ticket, action)
	}
	if action == "" {
		return payload, resolutionType
	}
	ret := CloneDecisionPayload(payload)
	if ret == nil {
		ret = map[string]any{}
	}
	ret["recoveryAction"] = action
	switch action {
	case "replan":
		return ret, decisioncommon.ResolutionTypeReplan
	case "terminate":
		return ret, decisioncommon.ResolutionTypeRejectCurrentPath
	default:
		return ret, decisioncommon.ResolutionTypeProvideMissingInfo
	}
}

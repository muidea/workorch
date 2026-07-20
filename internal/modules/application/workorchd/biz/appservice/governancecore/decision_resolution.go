package governancecore

import (
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func NormalizeDecisionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload workorcheventpayload.Payload, resolutionType string) (workorcheventpayload.Payload, string) {
	payload = NormalizeCandidateSelectionResolutionPayloadForTicket(ticket, payload)
	if IsWorkorchRuntimeLLMProviderRecoveryTicket(ticket) {
		// A provider-recovery ticket cannot create a successor Run: its only
		// safe continuation is to retry the already-sealed Runtime V2 step after
		// the operator has repaired or confirmed the provider. Keep a user's
		// replan intent visible in durable payload, but normalize it before the
		// allowed-action gate so every client receives the actionable outcome
		// instead of an opaque 400 response.
		if resolutionType == decisioncommon.ResolutionTypeReplan {
			ret := CloneDecisionPayload(payload).Merge(workorcheventpayload.New(
				workorcheventpayload.StringField("requestedResolutionType", decisioncommon.ResolutionTypeReplan),
				workorcheventpayload.StringField("recoveryAction", "retry_same_step"),
				workorcheventpayload.StringField("resolutionNotice", "replan is not applicable to provider recovery; retrying the sealed step"),
			))
			return ret, decisioncommon.ResolutionTypeProvideMissingInfo
		}
		action := InferRuntimeRepairRecoveryAction(payload)
		if action == "retry_same_step" {
			ret := CloneDecisionPayload(payload).Merge(workorcheventpayload.New(workorcheventpayload.StringField("recoveryAction", action)))
			return ret, decisioncommon.ResolutionTypeProvideMissingInfo
		}
	}
	if ticket != nil && ticket.DecisionKind == "runtime_v2_governance" && resolutionType == decisioncommon.ResolutionTypeReplan && ticketAllowsRuntimeContinuation(ticket) {
		// Runtime V2 has no replan transition. When this particular ticket
		// permits its continue resolution, retain the operator's replan intent
		// for audit and submit the concrete continuation that the persisted
		// governance binding accepts. The Decision owner will commit that
		// binding once; Orchestrator owns the resulting singleflight re-drive.
		ret := CloneDecisionPayload(payload).Merge(workorcheventpayload.New(
			workorcheventpayload.StringField("requestedResolutionType", decisioncommon.ResolutionTypeReplan),
			workorcheventpayload.StringField("runtimeContinuationIntent", "resume"),
			workorcheventpayload.StringField("resolutionNotice", "replan intent normalized to the ticket's Runtime V2 continuation"),
		))
		return ret, decisioncommon.ResolutionTypeProvideMissingInfo
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
	ret := CloneDecisionPayload(payload).Merge(workorcheventpayload.New(workorcheventpayload.StringField("recoveryAction", action)))
	switch action {
	case "replan":
		return ret, decisioncommon.ResolutionTypeReplan
	case "terminate":
		return ret, decisioncommon.ResolutionTypeRejectCurrentPath
	default:
		return ret, decisioncommon.ResolutionTypeProvideMissingInfo
	}
}

func ticketAllowsRuntimeContinuation(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	for _, action := range ticket.AllowedRecoveryActions {
		if action == decisioncommon.ResolutionTypeProvideMissingInfo {
			return true
		}
	}
	return false
}

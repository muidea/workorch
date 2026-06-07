package activationcore

import "strings"

func ReplanContinuationFromGovernanceHistory(history []map[string]any) map[string]any {
	for idx := len(history) - 1; idx >= 0; idx-- {
		item := history[idx]
		if len(item) == 0 {
			continue
		}
		resolutionType := strings.TrimSpace(stringFromPayload(item, "resolutionType"))
		resolutionPayload := mapValue(item["resolutionPayload"])
		recoveryAction := strings.TrimSpace(stringFromPayload(resolutionPayload, "recoveryAction"))
		if resolutionType != "replan" && recoveryAction != "replan" {
			continue
		}
		replan := map[string]any{
			"predecessorRunId": stringFromPayload(item, "runId"),
			"reason":           "resolved_governance_replan",
			"ticketId":         stringFromPayload(item, "ticketId"),
			"ticketStepId":     stringFromPayload(item, "stepId"),
			"ticketReason":     stringFromPayload(item, "reason"),
			"resolutionId":     stringFromPayload(item, "resolutionId"),
			"resolutionType":   resolutionType,
		}
		if len(resolutionPayload) > 0 {
			replan["resolutionPayload"] = CompactDecisionPayload(resolutionPayload)
		}
		if failure := ReplanFailureContextFromGovernanceItem(item); len(failure) > 0 {
			replan["failureContext"] = failure
			replan["ticketContext"] = failure
		}
		return replan
	}
	return nil
}

func ReplanFailureContextFromGovernanceItem(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	context := mapValue(item["ticketContext"])
	if len(context) == 0 {
		context = mapValue(item["context"])
	}
	if len(context) > 0 {
		return CompactFailureContext(context)
	}
	ret := map[string]any{}
	for _, key := range []string{
		"failureClass",
		"failureSignature",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"repairAttempts",
		"failedStepOutput",
		"recoveryPolicy",
	} {
		if value, ok := item[key]; ok && value != nil {
			ret[key] = CompactContextValue(value, 1)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

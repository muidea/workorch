package activationcore

func CompactDecisionPayload(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"answer",
		"note",
		"operator",
		"recoveryAction",
		"resumeInstruction",
		"resolution",
		"summary",
		"reason",
		"failureClass",
		"failureSignature",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"attempts",
		"maxAttempts",
		"repairOfRootStepId",
		"recoveryPolicy",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = CompactContextValue(value, 1)
		}
	}
	for _, key := range []string{"failureContext", "ticketContext", "context"} {
		if context := mapValue(input[key]); len(context) > 0 {
			ret[key] = CompactFailureContext(context)
		}
	}
	return StripRuntimeLLMBinding(ret)
}

func CompactFailureContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"failureClass",
		"failureSignature",
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"failureReason",
		"repairObjective",
		"attempts",
		"maxAttempts",
		"repairAttempts",
		"repairOfRootStepId",
		"runtimeRepairAttempt",
		"maxRuntimeRepairAttempts",
		"recoveryPolicy",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = CompactContextValue(value, 1)
		}
	}
	if failedOutput := mapValue(input["failedStepOutput"]); len(failedOutput) > 0 {
		ret["failedStepOutput"] = CompactFailedOutput(failedOutput)
	}
	if targets := CompactTargetSnapshots(input); len(targets) > 0 {
		ret["targetSnapshots"] = targets
	}
	return StripRuntimeLLMBinding(ret)
}

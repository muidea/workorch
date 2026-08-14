package readmodelprojection

func projectStepContractAudit(sources ...map[string]any) map[string]any {
	var input map[string]any
	for _, source := range sources {
		if len(source) == 0 {
			continue
		}
		if readModelStringValue(source["schemaVersion"]) == "workorch.step_contract_audit" ||
			len(readModelMapValue(source["requiredOutput"])) > 0 ||
			readModelStringValue(source["currentActionKind"]) != "" ||
			readModelStringValue(source["currentFocusId"]) != "" {
			if len(input) == 0 {
				input = source
				continue
			}
		}
		if len(readModelMapValue(source["currentActionContract"])) > 0 ||
			len(readModelMapValue(source["coverageWorkItemContext"])) > 0 ||
			len(readModelMapValue(source["convergenceAuthority"])) > 0 {
			input = source
			break
		}
	}
	if len(input) == 0 {
		return nil
	}
	if readModelStringValue(input["schemaVersion"]) == "workorch.step_contract_audit" {
		return cloneReadModelMap(input)
	}

	currentActionContract := readModelMapValue(input["currentActionContract"])
	coverageContext := readModelMapValue(input["coverageWorkItemContext"])
	authority := readModelMapValue(input["convergenceAuthority"])
	if len(coverageContext) == 0 {
		coverageContext = readModelMapValue(authority["coverageWorkItemContext"])
	}
	requiredOutput, requiredOutputSource := selectStepContractRequiredOutput(input, currentActionContract)
	currentFocus := firstNonEmptyMap(
		readModelMapValue(currentActionContract["currentFocus"]),
		readModelMapValue(coverageContext["currentFocus"]),
		readModelMapValue(authority["currentFocus"]),
	)
	nextAction := firstNonEmptyMap(
		readModelMapValue(currentActionContract["nextAction"]),
		readModelMapValue(currentActionContract["prerequisite"]),
		readModelMapValue(authority["nextAction"]),
	)

	ret := map[string]any{
		"schemaVersion": "workorch.step_contract_audit",
	}
	if stage := firstNonEmptyString(
		readModelStringValue(input["runtimeStage"]),
		readModelStringValue(currentActionContract["runtimeStage"]),
	); stage != "" {
		ret["runtimeStage"] = stage
	}
	if kind := firstNonEmptyString(
		readModelStringValue(nextAction["kind"]),
		readModelStringValue(nextAction["type"]),
		readModelStringValue(requiredOutput["nextActionKind"]),
		readModelStringValue(requiredOutput["actionKind"]),
	); kind != "" {
		ret["currentActionKind"] = kind
	}
	if focusID := firstNonEmptyString(
		focusIDFromMap(currentFocus),
		readModelStringValue(requiredOutput["coverageFocusId"]),
		readModelStringValue(requiredOutput["focusId"]),
	); focusID != "" {
		ret["currentFocusId"] = focusID
	}
	if ids := compactUniqueStrings(append(readModelStringSliceValue(currentFocus["coverageWorkItemIds"]), readModelStringSliceValue(requiredOutput["coverageWorkItemIds"])...)); len(ids) > 0 {
		ret["coverageWorkItemIds"] = ids
	}
	if kinds := compactUniqueStrings(firstNonEmptyStringSlice(
		readModelStringSliceValue(currentFocus["missingEvidenceKinds"]),
		readModelStringSliceValue(requiredOutput["missingEvidenceKinds"]),
		readModelStringSliceValue(currentFocus["requiredEvidenceKinds"]),
		readModelStringSliceValue(requiredOutput["requiredEvidenceKinds"]),
	)); len(kinds) > 0 {
		ret["missingEvidenceKinds"] = kinds
	}
	if len(requiredOutput) > 0 {
		ret["requiredOutputSource"] = requiredOutputSource
		ret["requiredOutput"] = compactStepRequiredOutputForAudit(requiredOutput)
	}
	if diagnostic := selectStepContractValidationFailureDiagnostic(input, currentActionContract); len(diagnostic) > 0 {
		ret["validationFailureDiagnostic"] = diagnostic
	}
	return ret
}

func selectStepContractValidationFailureDiagnostic(input map[string]any, currentActionContract map[string]any) map[string]any {
	for _, source := range []map[string]any{
		input,
		readModelMapValue(input["runtimeDiagnostics"]),
		readModelMapValue(input["lastFailureDiagnostics"]),
		currentActionContract,
	} {
		if diagnostic := readModelMapValue(source["validationFailureDiagnostic"]); len(diagnostic) > 0 {
			return compactStepValidationFailureDiagnosticForAudit(diagnostic)
		}
	}
	return nil
}

func selectStepContractRequiredOutput(input map[string]any, currentActionContract map[string]any) (map[string]any, string) {
	candidates := []struct {
		name  string
		value map[string]any
	}{
		{name: "currentActionContract.requiredOutput", value: readModelMapValue(currentActionContract["requiredOutput"])},
		{name: "requiredOutput", value: readModelMapValue(input["requiredOutput"])},
		{name: "repairActionPackage.requiredOutput", value: readModelMapValue(readModelMapValue(input["repairActionPackage"])["requiredOutput"])},
	}
	for _, candidate := range candidates {
		if len(candidate.value) > 0 {
			return candidate.value, candidate.name
		}
	}
	return nil, ""
}

func compactStepValidationFailureDiagnosticForAudit(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"schemaVersion",
		"class",
		"recommendedEvidenceTargetType",
		"stage",
		"operation",
		"commandKind",
		"reason",
		"violatedRule",
		"failureSignature",
	} {
		if value := readModelStringValue(input[key]); value != "" {
			ret[key] = value
		}
	}
	for _, key := range []string{
		"commandPolicyViolation",
		"stateChangingEvidenceNeeded",
		"implementationGap",
	} {
		if value, ok := input[key].(bool); ok {
			ret[key] = value
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func compactStepRequiredOutputForAudit(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"schemaVersion",
		"kind",
		"mode",
		"expectedState",
		"currentState",
		"coverageFocusId",
		"focusId",
		"nextActionKind",
		"actionKind",
	} {
		if value := readModelStringValue(input[key]); value != "" {
			ret[key] = value
		}
	}
	for _, key := range []string{
		"coverageWorkItemIds",
		"acceptedKinds",
		"allowedKinds",
		"forbiddenKinds",
		"mustNotProduceKinds",
		"requiredEvidenceKinds",
		"missingEvidenceKinds",
		"structuredTargetRefs",
		"targetRefs",
	} {
		if values := compactUniqueStrings(readModelStringSliceValue(input[key])); len(values) > 0 {
			ret[key] = values
		}
	}
	for _, key := range []string{
		"mustProduceConcreteValidationEvidence",
		"mustProduceStateChange",
		"mustReturnExecutionGraph",
		"mustReturnValidationEvidence",
		"mustReturnArtifactChunk",
		"mustReturnArtifactContextGraph",
		"assessmentAllowed",
		"decisionAllowed",
	} {
		if value, ok := input[key].(bool); ok {
			ret[key] = value
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

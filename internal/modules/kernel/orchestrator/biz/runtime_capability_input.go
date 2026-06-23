package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func capabilityInputLocatorInvalidFailure(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	return capabilityInputLocatorInvalidOutput(step.Output)
}

func capabilityInputLocatorInvalidOutput(output map[string]any) bool {
	if len(output) == 0 || !externalCapabilityFailureHasCallIdentity(output) {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
		stringFromMap(output, "stderr"),
		stringFromMap(output, "stdout"),
		stringFromMap(output, failureOutputKeySignature),
		stringFromMap(mapFromValue(output["runtimeDiagnostics"]), "error"),
		stringFromMap(mapFromValue(output["runtimeDiagnostics"]), "reason"),
	}, "\n"))
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, marker := range []string{
		"column is beyond end of line",
		"line is beyond end of file",
		"line is out of range",
		"column is out of range",
		"position is out of range",
		"offset is out of range",
		"range is out of",
		"location out of bounds",
		"invalid line",
		"invalid column",
		"invalid position",
		"invalid locator",
		"target locator invalid",
		"not a valid position",
		"no identifier found",
		"no symbol at position",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func inputHasCapabilityInputLocatorInvalid(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if boolValue(input["capabilityInputLocatorInvalid"]) {
		return true
	}
	if boolValue(mapFromValue(input["capabilityInputFailure"])["locatorInvalid"]) {
		return true
	}
	if signals := mapFromValue(input["structuredFailureSignals"]); boolValue(signals["capabilityInputLocatorInvalid"]) {
		return true
	}
	return capabilityInputLocatorInvalidOutput(mapFromValue(input["failedStepOutput"]))
}

func repairContextHasCapabilityInputLocatorInvalid(input map[string]any, pkg map[string]any) bool {
	return inputHasCapabilityInputLocatorInvalid(input) ||
		boolValue(pkg["capabilityInputLocatorInvalid"]) ||
		boolValue(mapFromValue(pkg["capabilityInputFailure"])["locatorInvalid"]) ||
		boolValue(mapFromValue(pkg["structuredFailureSignals"])["capabilityInputLocatorInvalid"])
}

func markCapabilityInputLocatorEvidenceRequired(input map[string]any, failedStep *runModel.Step) {
	if len(input) == 0 || !capabilityInputLocatorInvalidFailure(failedStep) {
		return
	}
	input["capabilityInputLocatorInvalid"] = true
	input["repairObjective"] = repairObjectiveAcquireFailureEvidence
	input["concreteFailureEvidenceRequired"] = true
	input["evidenceAcquisitionReason"] = "capability_input_locator_invalid"
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"The failed capability call used an invalid locator or position. Treat the target refs as capability input scope, not as implementation repair evidence.",
		"Acquire bounded evidence to determine a valid locator, symbol, range, or equivalent capability input before any state-changing repair.",
		"Do not patch workspace resources merely to make a previously invalid capability input coordinate valid.",
	)
	delete(input, "stateChangingOutputRequired")
}

func capabilityInputLocatorEvidencePackage(input map[string]any, failedStep *runModel.Step) map[string]any {
	pkg := cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["capabilityInputLocatorInvalid"] = true
	pkg["evidenceAcquisitionReason"] = "capability_input_locator_invalid"
	pkg["evidenceAcquisitionRequired"] = true
	required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
	required["evidenceAcquisitionRule"] = "bounded evidence acquisition is required because the previous capability call used an invalid locator or input position"
	required["capabilityInputLocatorRule"] = "first obtain a valid locator, symbol, range, target snapshot, or equivalent capability input evidence; do not patch deliverables merely to make the rejected locator valid"
	pkg["requiredOutput"] = required
	if failedStep != nil {
		failure := map[string]any{
			"locatorInvalid": true,
			"failedStepId":   string(failedStep.ID),
			"failedStepKind": failedStep.Kind,
			"reason": firstNonEmptyString(
				stringFromMap(failedStep.Output, "error"),
				stringFromMap(failedStep.Output, "reason"),
			),
		}
		if callInput := mapFromValue(failedStep.Output["input"]); len(callInput) > 0 {
			failure["input"] = compactLLMContextMap(callInput, 1)
		}
		if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
			failure["runtimeDiagnostics"] = compactLLMContextMap(diagnostics, 1)
		}
		pkg["capabilityInputFailure"] = failure
	}
	stripRepairActionPackageImplementationTargetEvidence(pkg)
	sanitizeRepairActionPackageTargetEvidence(pkg)
	return pkg
}

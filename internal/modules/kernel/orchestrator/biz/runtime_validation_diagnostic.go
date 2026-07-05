package orchestrator

import (
	"context"
	"errors"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validationFailureDiagnosticFromStep(step *runModel.Step, execErr error) map[string]any {
	if step == nil || len(step.Output) == 0 {
		return nil
	}
	if diagnostic := normalizeValidationFailureDiagnostic(mapFromValue(step.Output["validationFailureDiagnostic"])); len(diagnostic) > 0 {
		return enrichValidationFailureDiagnosticFromStep(diagnostic, step, execErr)
	}
	if diagnostic := normalizeValidationFailureDiagnostic(mapFromValue(mapFromValue(step.Output["runtimeDiagnostics"])["validationFailureDiagnostic"])); len(diagnostic) > 0 {
		return enrichValidationFailureDiagnosticFromStep(diagnostic, step, execErr)
	}
	if strings.TrimSpace(step.Kind) != "shell/command" {
		return nil
	}
	diagnostic := classifyShellValidationFailureDiagnostic(step, execErr)
	if len(diagnostic) == 0 {
		return nil
	}
	return enrichValidationFailureDiagnosticFromStep(diagnostic, step, execErr)
}

func classifyShellValidationFailureDiagnostic(step *runModel.Step, execErr error) map[string]any {
	if step == nil || len(step.Output) == 0 {
		return nil
	}
	output := step.Output
	validationEvidence := mapFromValue(firstNonNilValue(output["validationEvidence"], step.Input["validationEvidence"]))
	class, targetType := validationFailureClassAndTargetFromEvidence(validationEvidence)
	switch {
	case strings.EqualFold(stringFromMap(output, "status"), "timeout") || errors.Is(execErr, context.DeadlineExceeded):
		class = runtimemodel.ValidationDiagnosticTimeout
		targetType = runtimemodel.ValidationEvidenceTargetValidationOutput
	case boolValue(mapFromValue(output["runtimeDiagnostics"])["commandPolicyViolation"]):
		class = runtimemodel.ValidationDiagnosticCommandInvalid
		targetType = runtimemodel.ValidationEvidenceTargetCommandContract
	case strings.TrimSpace(stringFromMap(output, "unavailableCommand")) != "" || boolValue(output["capabilityUnavailable"]):
		class = runtimemodel.ValidationDiagnosticEnvironmentMissing
		targetType = runtimemodel.ValidationEvidenceTargetEnvironment
	case shellValidationFailureLooksExternalDependencyUnavailable(output, execErr):
		class = runtimemodel.ValidationDiagnosticExternalDependency
		targetType = runtimemodel.ValidationEvidenceTargetDependencyContract
	case class != "":
	case boolValue(output["zeroWork"]) || strings.TrimSpace(stringFromMap(output, "evidenceQuality")) == string(commonEvidenceQualityInconclusive()):
		class = runtimemodel.ValidationDiagnosticOutputInconclusive
		targetType = runtimemodel.ValidationEvidenceTargetValidationOutput
	case int64FromValue(output["exitCode"]) != 0:
		class, targetType = validationFailureClassAndTargetFromObservation(validationEvidence)
	default:
		return nil
	}
	return map[string]any{
		"schemaVersion":                 runtimemodel.ValidationFailureDiagnosticSchemaVersion,
		"class":                         class,
		"reason":                        firstNonEmptyString(stringFromMap(output, "reason"), stringFromMap(output, "error"), errorString(execErr), stringFromMap(output, "evidenceReason")),
		"recommendedEvidenceTargetType": targetType,
		"stage":                         firstNonEmptyString(stringFromMap(output, "stage"), "shell_command_failed"),
		"operation":                     "shell/command",
		"commandKind":                   "shell/command",
	}
}

func enrichValidationFailureDiagnosticFromStep(diagnostic map[string]any, step *runModel.Step, execErr error) map[string]any {
	if len(diagnostic) == 0 || step == nil {
		return nil
	}
	ret := cloneStepPayload(diagnostic)
	if strings.TrimSpace(stringFromMap(ret, "schemaVersion")) == "" {
		ret["schemaVersion"] = runtimemodel.ValidationFailureDiagnosticSchemaVersion
	}
	if shellValidationFailureLooksExternalDependencyUnavailable(step.Output, execErr) &&
		validationDiagnosticClassAllowsExternalDependencyPromotion(stringFromMap(ret, "class")) {
		ret["class"] = runtimemodel.ValidationDiagnosticExternalDependency
		ret["recommendedEvidenceTargetType"] = runtimemodel.ValidationEvidenceTargetDependencyContract
	}
	if strings.TrimSpace(stringFromMap(ret, "reason")) == "" {
		ret["reason"] = firstNonEmptyString(stringFromMap(step.Output, "reason"), stringFromMap(step.Output, "error"), errorString(execErr))
	}
	if strings.TrimSpace(stringFromMap(ret, "operation")) == "" {
		ret["operation"] = step.Kind
	}
	if strings.TrimSpace(stringFromMap(ret, "commandKind")) == "" && step.Kind != "" {
		ret["commandKind"] = step.Kind
	}
	if strings.TrimSpace(stringFromMap(ret, "stage")) == "" {
		ret["stage"] = stringFromMap(step.Output, "stage")
	}
	if _, ok := ret["exitCode"]; !ok {
		if _, ok := step.Output["exitCode"]; ok {
			ret["exitCode"] = step.Output["exitCode"]
		}
	}
	if refs := validationFailureDiagnosticTargetRefs(step, ret); len(refs) > 0 {
		ret["targetRefs"] = refs
	}
	if evidence := mapFromValue(firstNonNilValue(step.Output["validationEvidence"], step.Input["validationEvidence"])); len(evidence) > 0 {
		if strings.TrimSpace(stringFromMap(ret, "observationType")) == "" {
			ret["observationType"] = firstNonEmptyString(stringFromMap(evidence, "observationType"), stringFromMap(evidence, "observation"))
		}
		if strings.TrimSpace(stringFromMap(ret, "evidenceMode")) == "" {
			ret["evidenceMode"] = stringFromMap(evidence, "mode")
		}
	}
	if stderr := strings.TrimSpace(stringFromMap(step.Output, "stderr")); stderr != "" && strings.TrimSpace(stringFromMap(ret, "stderrPreview")) == "" {
		ret["stderrPreview"] = truncateLLMEvidenceText(stderr)
	}
	if stdout := strings.TrimSpace(stringFromMap(step.Output, "stdout")); stdout != "" && strings.TrimSpace(stringFromMap(ret, "stdoutPreview")) == "" {
		ret["stdoutPreview"] = truncateLLMEvidenceText(stdout)
	}
	return compactLLMContextMap(ret, 1)
}

func validationDiagnosticClassAllowsExternalDependencyPromotion(class string) bool {
	switch strings.TrimSpace(class) {
	case "",
		runtimemodel.ValidationDiagnosticBuildFailed,
		runtimemodel.ValidationDiagnosticTestFailed,
		runtimemodel.ValidationDiagnosticDependencyMismatch,
		runtimemodel.ValidationDiagnosticTimeout,
		runtimemodel.ValidationDiagnosticOutputInconclusive:
		return true
	default:
		return false
	}
}

func shellValidationFailureLooksExternalDependencyUnavailable(output map[string]any, execErr error) bool {
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		stringFromMap(output, "stderr"),
		stringFromMap(output, "stdout"),
		stringFromMap(output, "stderrPreview"),
		stringFromMap(output, "stdoutPreview"),
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
		stringFromMap(mapFromValue(output["validationFailureDiagnostic"]), "stderrPreview"),
		stringFromMap(mapFromValue(output["validationFailureDiagnostic"]), "stdoutPreview"),
		stringFromMap(mapFromValue(mapFromValue(output["runtimeDiagnostics"])["validationFailureDiagnostic"]), "stderrPreview"),
		stringFromMap(mapFromValue(mapFromValue(output["runtimeDiagnostics"])["validationFailureDiagnostic"]), "stdoutPreview"),
		errorString(execErr),
	}, "\n")))
	if text == "" {
		return false
	}
	hasNetworkSurface := false
	for _, marker := range []string{
		"i/o timeout",
		"network is unreachable",
		"no route to host",
		"temporary failure in name resolution",
		"connection timed out",
		"connection timeout",
		"tls handshake timeout",
		"proxyconnect tcp",
		"dial tcp",
		"dial udp",
		"lookup ",
		"could not resolve host",
		"unable to resolve host",
	} {
		if strings.Contains(text, marker) {
			hasNetworkSurface = true
			break
		}
	}
	if !hasNetworkSurface {
		return false
	}
	for _, marker := range []string{
		"http://",
		"https://",
		"fetch",
		"download",
		"module",
		"package",
		"dependency",
		"registry",
		"repository",
		"remote",
		"provider",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func normalizeValidationFailureDiagnostic(diagnostic map[string]any) map[string]any {
	if len(diagnostic) == 0 {
		return nil
	}
	class := allowedValidationDiagnosticClass(stringFromMap(diagnostic, "class"))
	if class == "" {
		return nil
	}
	ret := cloneStepPayload(diagnostic)
	ret["class"] = class
	if strings.TrimSpace(stringFromMap(ret, "recommendedEvidenceTargetType")) == "" {
		ret["recommendedEvidenceTargetType"] = defaultValidationDiagnosticTargetType(class)
	}
	return compactLLMContextMap(ret, 1)
}

func validationFailureClassAndTargetFromEvidence(evidence map[string]any) (string, string) {
	class := allowedValidationDiagnosticClass(firstNonEmptyString(
		stringFromMap(evidence, "diagnosticClass"),
		stringFromMap(evidence, "failureClass"),
		stringFromMap(evidence, "expectedFailureClass"),
	))
	if class == "" {
		return "", ""
	}
	return class, firstNonEmptyString(stringFromMap(evidence, "recommendedEvidenceTargetType"), defaultValidationDiagnosticTargetType(class))
}

func validationFailureClassAndTargetFromObservation(evidence map[string]any) (string, string) {
	switch strings.TrimSpace(firstNonEmptyString(stringFromMap(evidence, "observationType"), stringFromMap(evidence, "observation"))) {
	case "dependency_resolution", "dependency_check", "dependency_contract":
		return runtimemodel.ValidationDiagnosticDependencyMismatch, runtimemodel.ValidationEvidenceTargetDependencyContract
	case "toolchain_resolution", "toolchain_check", "toolchain_contract":
		return runtimemodel.ValidationDiagnosticToolchainMismatch, runtimemodel.ValidationEvidenceTargetToolchainContract
	case "test_result", "assertion_result", "behavior_test_result":
		return runtimemodel.ValidationDiagnosticTestFailed, runtimemodel.ValidationEvidenceTargetBehaviorEvidence
	case "build_result", "build_observation", "executable_contract_observation":
		return runtimemodel.ValidationDiagnosticBuildFailed, runtimemodel.ValidationEvidenceTargetFailureSurface
	default:
		return runtimemodel.ValidationDiagnosticBuildFailed, runtimemodel.ValidationEvidenceTargetFailureSurface
	}
}

func allowedValidationDiagnosticClass(value string) string {
	switch strings.TrimSpace(value) {
	case runtimemodel.ValidationDiagnosticCommandInvalid,
		runtimemodel.ValidationDiagnosticEnvironmentMissing,
		runtimemodel.ValidationDiagnosticDependencyMismatch,
		runtimemodel.ValidationDiagnosticToolchainMismatch,
		runtimemodel.ValidationDiagnosticBuildFailed,
		runtimemodel.ValidationDiagnosticTestFailed,
		runtimemodel.ValidationDiagnosticTimeout,
		runtimemodel.ValidationDiagnosticOutputInconclusive,
		runtimemodel.ValidationDiagnosticEvidenceBindingMissing,
		runtimemodel.ValidationDiagnosticContractViolation,
		runtimemodel.ValidationDiagnosticEvidenceMetadataMissing,
		runtimemodel.ValidationDiagnosticScratchRequired,
		runtimemodel.ValidationDiagnosticScratchEscape,
		runtimemodel.ValidationDiagnosticProcessControlForbidden,
		runtimemodel.ValidationDiagnosticDetachedProcessForbidden,
		runtimemodel.ValidationDiagnosticShellSafetyUnproven,
		runtimemodel.ValidationDiagnosticManagedValidationRequired,
		runtimemodel.ValidationDiagnosticImplementationGap,
		runtimemodel.ValidationDiagnosticExternalDependency,
		runtimemodel.ValidationDiagnosticProviderTransient,
		runtimemodel.ValidationDiagnosticRuntimeMisalignment:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func defaultValidationDiagnosticTargetType(class string) string {
	switch strings.TrimSpace(class) {
	case runtimemodel.ValidationDiagnosticCommandInvalid:
		return runtimemodel.ValidationEvidenceTargetCommandContract
	case runtimemodel.ValidationDiagnosticEnvironmentMissing:
		return runtimemodel.ValidationEvidenceTargetEnvironment
	case runtimemodel.ValidationDiagnosticDependencyMismatch:
		return runtimemodel.ValidationEvidenceTargetDependencyContract
	case runtimemodel.ValidationDiagnosticToolchainMismatch:
		return runtimemodel.ValidationEvidenceTargetToolchainContract
	case runtimemodel.ValidationDiagnosticEvidenceBindingMissing:
		return runtimemodel.ValidationEvidenceTargetCoverageBinding
	case runtimemodel.ValidationDiagnosticTimeout, runtimemodel.ValidationDiagnosticOutputInconclusive:
		return runtimemodel.ValidationEvidenceTargetValidationOutput
	case runtimemodel.ValidationDiagnosticTestFailed:
		return runtimemodel.ValidationEvidenceTargetBehaviorEvidence
	case runtimemodel.ValidationDiagnosticContractViolation,
		runtimemodel.ValidationDiagnosticEvidenceMetadataMissing,
		runtimemodel.ValidationDiagnosticScratchRequired,
		runtimemodel.ValidationDiagnosticScratchEscape,
		runtimemodel.ValidationDiagnosticProcessControlForbidden,
		runtimemodel.ValidationDiagnosticDetachedProcessForbidden,
		runtimemodel.ValidationDiagnosticShellSafetyUnproven,
		runtimemodel.ValidationDiagnosticManagedValidationRequired:
		return runtimemodel.ValidationEvidenceTargetCommandContract
	case runtimemodel.ValidationDiagnosticImplementationGap:
		return runtimemodel.ValidationEvidenceTargetFailureSurface
	case runtimemodel.ValidationDiagnosticExternalDependency:
		return runtimemodel.ValidationEvidenceTargetDependencyContract
	case runtimemodel.ValidationDiagnosticProviderTransient:
		return runtimemodel.ValidationEvidenceTargetEnvironment
	case runtimemodel.ValidationDiagnosticRuntimeMisalignment:
		return runtimemodel.ValidationEvidenceTargetValidationOutput
	default:
		return runtimemodel.ValidationEvidenceTargetFailureSurface
	}
}

func validationFailureDiagnosticTargetRefs(step *runModel.Step, diagnostic map[string]any) []string {
	seen := map[string]bool{}
	ret := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, ref := range stringSliceFromValue(diagnostic["targetRefs"]) {
		add(ref)
	}
	for _, ref := range stringSliceFromValue(step.Output["targetRefs"]) {
		add(ref)
	}
	for _, ref := range failureTargetsForSignature(step.Output) {
		add(ref)
	}
	for _, evidence := range []map[string]any{mapFromValue(step.Input["validationEvidence"]), mapFromValue(step.Output["validationEvidence"])} {
		add(stringFromMap(evidence, "targetRef"))
		for _, ref := range stringSliceFromValue(evidence["targetRefs"]) {
			add(ref)
		}
		for _, ref := range stringSliceFromValue(evidence["coverageTargets"]) {
			add(ref)
		}
	}
	return limitedStrings(ret, 5)
}

func commonEvidenceQualityInconclusive() string {
	return "inconclusive"
}

func firstNonNilValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func runtimeFailureDecisionDiagnostics(step *runModel.Step, execErr error) map[string]any {
	if step == nil {
		return nil
	}
	class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature))
	signals := structuredFailureSignalsFromFailureOutput(step.Output, execErr, class, signature)
	externalDependency := failedStepHasExternalRuntimeDependency(step)
	if len(signals) == 0 && !externalDependency {
		return nil
	}
	ret := map[string]any{
		"recommendedNextActions": runtimeFailureRecommendedNextActions(signals),
	}
	if len(signals) > 0 {
		ret["structuredFailureSignals"] = compactStructuredFailureSignalsForRepairActionPackage(signals)
	}
	if externalDependency {
		ret["externalDependencyUnavailable"] = true
		ret["requiresExternalDependency"] = true
		ret["recommendedNextActions"] = []string{
			"Confirm the external dependency, network route, proxy, credential, or local cache required by the failed validation.",
			"Provide missing runtime configuration or retry after the dependency is reachable.",
			"Do not repeat the same state-changing or validation command until the external dependency surface changes.",
		}
	}
	if primary := primaryDiagnosticRepairContractFromSignals(signals); len(primary) > 0 {
		ret["primaryDiagnosticRepair"] = compactLLMContextMap(primary, 1)
	}
	return ret
}

func runtimeFailureRecommendedNextActions(signals map[string]any) []string {
	if structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
		return []string{
			"Read the current failure target resources or use provided target snapshots before patching.",
			"Patch the smallest target set that addresses the line-referenced diagnostics.",
			"Rerun the failing validation command after the patch.",
			"If the target diagnostics conflict with current files, provide focused missing information instead of retrying the same repair.",
		}
	}
	return []string{
		"Identify the smallest concrete target set before applying another state-changing repair.",
		"Collect bounded validation evidence for the failed step before retrying broad repair.",
		"If key constraints remain unclear, provide focused missing information instead of repeating the same repair.",
	}
}

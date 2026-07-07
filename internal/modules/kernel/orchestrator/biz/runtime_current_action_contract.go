package orchestrator

import (
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	currentActionContractKey    = "currentActionContract"
	currentActionContractSchema = "workorch.current_action_contract.v1"
)

func attachCurrentActionContractToInput(run *runModel.Run, stepID common.StepID, input map[string]any) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	input = normalizeForcedNarrowManagedValidationInput(input)
	contract := currentActionContractFromInput(run, stepID, input)
	if len(contract) == 0 {
		delete(input, currentActionContractKey)
		return input
	}
	input[currentActionContractKey] = contract
	return input
}

func currentActionContractFromInput(run *runModel.Run, stepID common.StepID, input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	nextAction := currentActionNextActionFromInput(input)
	required := activeRequiredOutputForCurrentAction(input)
	if len(nextAction) == 0 && len(required) == 0 {
		return nil
	}
	ret := map[string]any{
		"schemaVersion": currentActionContractSchema,
	}
	if run != nil {
		ret["workspaceId"] = string(run.WorkspaceID)
		ret["runId"] = string(run.ID)
	}
	if stepID != "" {
		ret["stepId"] = string(stepID)
	}
	if len(nextAction) > 0 {
		ret["nextAction"] = cloneStepPayload(nextAction)
	}
	if prerequisite := currentActionPrerequisiteFromInput(input); len(prerequisite) > 0 {
		ret["prerequisite"] = cloneStepPayload(prerequisite)
	}
	if focus := dispatchAuthorityCurrentFocus(input); len(focus) > 0 {
		ret["currentFocus"] = cloneStepPayload(focus)
	}
	if len(required) > 0 {
		ret["requiredOutput"] = cloneStepPayload(required)
	}
	if snapshots := currentActionTargetSnapshotRefsFromInput(input); len(snapshots) > 0 {
		ret["targetSnapshots"] = snapshots
	}
	for _, key := range []string{
		"runtimeStage",
		"runtimeStageMode",
		"currentRuntimeStage",
		"currentRuntimeStageMode",
		"repairObjective",
		"stateChangingOutputRequired",
		"concreteFailureEvidenceRequired",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"coverageRepairRequired",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = cloneStepInputValue(value)
		}
	}
	if recovery := mapFromValue(input["contractViolationRecovery"]); len(recovery) > 0 {
		ret["latestRejectedDiagnostic"] = compactCurrentActionDiagnostic(recovery)
	}
	return ret
}

func currentActionTargetSnapshotRefsFromInput(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	if strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind")) != runtimemodel.NextActionTypeStateChange {
		return nil
	}
	pkg := mapFromValue(input["repairActionPackage"])
	snapshots := mergeRepairTargetSnapshots(
		mapSliceFromValue(pkg["targetSnapshots"]),
		mapSliceFromValue(pkg["currentTargetSnapshots"]),
	)
	snapshots = mergeRepairTargetSnapshots(snapshots, repairActionTargetSnapshots(input))
	exact := exactPatchSafeRepairTargetSnapshots(snapshots)
	if len(exact) == 0 {
		return nil
	}
	return repairActionTargetSnapshotMetadata(exact)
}

func currentActionNextActionFromInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, source := range []map[string]any{
		mapFromValue(input[convergenceAuthorityKey]),
		mapFromValue(input[gapBaselineKey]),
		mapFromValue(mapFromValue(input["executionContext"])[convergenceAuthorityKey]),
		mapFromValue(mapFromValue(input["executionContext"])[gapBaselineKey]),
	} {
		if nextAction := mapFromValue(source["nextAction"]); len(nextAction) > 0 {
			return cloneStepPayload(nextAction)
		}
	}
	if nextAction := currentActionNextActionFromConvergenceContract(mapFromValue(input["convergenceContract"])); len(nextAction) > 0 {
		return nextAction
	}
	return nil
}

func currentActionNextActionFromConvergenceContract(contract map[string]any) map[string]any {
	if len(contract) == 0 {
		return nil
	}
	if boolValue(contract["deterministicComplete"]) {
		return map[string]any{"kind": "complete", "reason": firstNonEmptyString(stringFromMap(contract, "completionReason"), "current convergence contract is deterministically satisfied")}
	}
	state := strings.TrimSpace(stringFromMap(contract, "state"))
	switch state {
	case convergenceStateNeedEvidence:
		if strings.TrimSpace(stringFromMap(contract, "runtimeStageMode")) == "target_snapshot_acquisition" {
			return map[string]any{"kind": "acquire_target_snapshot", "reason": "current convergence contract requires target snapshot evidence before state change"}
		}
		return map[string]any{"kind": "acquire_evidence", "reason": "current convergence contract requires bounded evidence before continuing"}
	case convergenceStateNeedStateChange:
		if strings.TrimSpace(stringFromMap(contract, "runtimeStageMode")) == "target_snapshot_acquisition" {
			return map[string]any{"kind": "acquire_target_snapshot", "reason": "state_change requires current target snapshot prerequisite"}
		}
		next := map[string]any{"kind": "state_change", "reason": "current convergence contract requires state-changing output"}
		if prerequisite := mapFromValue(contract["prerequisite"]); len(prerequisite) > 0 {
			next["prerequisite"] = cloneStepPayload(prerequisite)
			next["reason"] = "state_change requires current prerequisite"
		}
		return next
	case convergenceStateNeedValidation:
		return map[string]any{"kind": "validate", "reason": "current convergence contract requires bounded validation evidence"}
	case convergenceStateNeedAssessment:
		return map[string]any{"kind": "assessment", "reason": "current convergence contract requires bounded coverage assessment"}
	case convergenceStateNeedGovernance:
		return map[string]any{"kind": "request_governance", "reason": firstNonEmptyString(stringFromMap(contract, "governanceReason"), "current convergence contract requires governance")}
	default:
		return nil
	}
}

func currentActionPrerequisiteFromInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, source := range []map[string]any{
		mapFromValue(input["convergenceContract"]),
		mapFromValue(input["repairActionPackage"]),
	} {
		if prerequisite := mapFromValue(source["prerequisite"]); len(prerequisite) > 0 {
			return cloneStepPayload(prerequisite)
		}
	}
	if prerequisite := mapFromValue(mapFromValue(input[currentActionContractKey])["prerequisite"]); len(prerequisite) > 0 {
		return cloneStepPayload(prerequisite)
	}
	contract := mapFromValue(input["convergenceContract"])
	required := activeRequiredOutputForCurrentAction(input)
	if strings.TrimSpace(stringFromMap(contract, "runtimeStageMode")) == "target_snapshot_acquisition" &&
		boolValue(required["mustAcquireCurrentTargetSnapshot"]) &&
		strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind")) == "state_change" {
		return targetSnapshotAcquisitionPrerequisite()
	}
	return nil
}

func activeRequiredOutputForCurrentAction(input map[string]any) map[string]any {
	if required := forcedNarrowManagedValidationRequiredOutput(input); len(required) > 0 {
		return normalizeCurrentAuthorityRequiredOutput(input, required)
	}
	switch state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state")); state {
	case convergenceStateNeedGovernance:
		return nil
	case convergenceStateNeedAssessment:
		return assessmentRequiredOutputForAuthorityFocus(dispatchCoverageFocusFromAuthority(input, dispatchAuthorityCurrentFocus(input)))
	case convergenceStateNeedEvidence, convergenceStateNeedStateChange, convergenceStateNeedValidation:
		return currentActionRequiredOutputForState(input, state)
	}
	if required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]); len(required) > 0 {
		return normalizeCurrentAuthorityRequiredOutput(input, required)
	}
	if required := mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]); len(required) > 0 {
		return normalizeCurrentAuthorityRequiredOutput(input, required)
	}
	if required := mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]); len(required) > 0 {
		return normalizeCurrentAuthorityRequiredOutput(input, required)
	}
	if required := mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["requiredOutput"]); len(required) > 0 {
		return normalizeCurrentAuthorityRequiredOutput(input, required)
	}
	if required := mapFromValue(mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyStagePacket])["requiredOutput"]); len(required) > 0 {
		return normalizeCurrentAuthorityRequiredOutput(input, required)
	}
	if required := mapFromValue(input["requiredOutput"]); len(required) > 0 {
		return normalizeCurrentAuthorityRequiredOutput(input, required)
	}
	return nil
}

func forcedNarrowManagedValidationRequiredOutput(input map[string]any) map[string]any {
	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if len(recovery) == 0 ||
		stringFromMap(recovery, "correctionMode") != contractViolationCorrectionModeForcedNarrow ||
		stringFromMap(mapFromValue(recovery["correctionContract"]), "requiredOutputShape") != "managed_validation_evidence" {
		return nil
	}
	return forcedNarrowRequiredOutputFromRecovery(input, nil, recovery)
}

func currentActionRequiredOutputForState(input map[string]any, state string) map[string]any {
	pkg := cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if pkg == nil {
		pkg = map[string]any{}
	}
	required := currentActionRequiredOutputSeed(input, state)
	if len(required) > 0 {
		pkg["requiredOutput"] = cloneStepPayload(required)
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	if nextAction == runtimemodel.NextActionTypeAcquireTargetSnapshot {
		next := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, required)
		applyTargetSnapshotAcquisitionRequiredOutputContract(next)
		return normalizeCurrentAuthorityRequiredOutput(input, next)
	}
	switch state {
	case convergenceStateNeedEvidence:
		next := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, required)
		if boolValue(required["mustAcquireCurrentTargetSnapshot"]) {
			applyTargetSnapshotAcquisitionRequiredOutputContract(next)
		}
		return normalizeCurrentAuthorityRequiredOutput(input, next)
	case convergenceStateNeedStateChange:
		if targetSnapshotPrerequisiteActive(input) && boolValue(required["mustAcquireCurrentTargetSnapshot"]) {
			next := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, required)
			applyTargetSnapshotAcquisitionRequiredOutputContract(next)
			return normalizeCurrentAuthorityRequiredOutput(input, next)
		}
		return normalizeCurrentAuthorityRequiredOutput(input, enforceStateChangingRequiredOutputForConvergence(pkg, input))
	case convergenceStateNeedValidation:
		return normalizeCurrentAuthorityRequiredOutput(input, concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, validationRequiredOutputSeed(required)))
	default:
		return nil
	}
}

func currentActionRequiredOutputSeed(input map[string]any, state string) map[string]any {
	for _, required := range []map[string]any{
		mapFromValue(mapFromValue(mapFromValue(input["convergenceContract"])["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		mapFromValue(input["requiredOutput"]),
		mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
	} {
		if currentActionRequiredOutputSeedMatchesState(required, state) {
			return cloneStepPayload(required)
		}
	}
	return nil
}

func currentActionRequiredOutputSeedMatchesState(required map[string]any, state string) bool {
	if len(required) == 0 {
		return false
	}
	switch state {
	case convergenceStateNeedValidation:
		return len(validationRequiredOutputSeed(required)) > 0
	case convergenceStateNeedStateChange:
		return requiredOutputRequiresStateChanging(required) || boolValue(required["mustAcquireCurrentTargetSnapshot"])
	case convergenceStateNeedEvidence:
		return !requiredOutputRequiresStateChanging(required)
	default:
		return true
	}
}

func validationRequiredOutputSeed(required map[string]any) map[string]any {
	if len(required) == 0 {
		return nil
	}
	if requiredOutputRequiresStateChanging(required) || requiredOutputRequiresConcreteFailureEvidence(required) {
		return nil
	}
	return required
}

func normalizeCurrentAuthorityRequiredOutput(input map[string]any, required map[string]any) map[string]any {
	if len(required) == 0 {
		return nil
	}
	authorityFocus := dispatchAuthorityCurrentFocus(input)
	if len(authorityFocus) == 0 {
		return normalizeRequiredOutputMCPToolContract(input, cloneStepPayload(required))
	}
	return normalizeRequiredOutputMCPToolContract(input, normalizeRequiredOutputFocusBinding(required, dispatchCoverageFocusFromAuthority(input, authorityFocus)))
}

func normalizeRequiredOutputMCPToolContract(input map[string]any, required map[string]any) map[string]any {
	if len(required) == 0 || !currentActionRequiresConcreteValidation(input) {
		return required
	}
	accepted := stringSliceFromValue(required["acceptedKinds"])
	if !stringSliceContains(accepted, runtimemodel.StepKindMCPToolCall) {
		return required
	}
	declared, _ := declaredRuntimeMCPServerIDsFromSources(
		input,
		mapFromValue(input["executionContext"]),
		mapFromValue(input[stepInputKeyExecutionContextPackage]),
		mapFromValue(input["runtimeCapabilities"]),
		mapFromValue(mapFromValue(input["executionContext"])["runtimeCapabilities"]),
	)
	if len(declared) > 0 {
		required["allowedMCPServerIds"] = sortedMCPServerIDs(declared)
		return required
	}
	required["acceptedKinds"] = removeStringsFromSlice(accepted, runtimemodel.StepKindMCPToolCall)
	required["mcpToolCallUnavailableReason"] = "mcp/tool_call is not accepted for this validation action because no concrete MCP server is declared in the current runtime capability context"
	return required
}

func sortedMCPServerIDs(declared map[string]bool) []string {
	ret := mapKeys(declared)
	sort.Strings(ret)
	return ret
}

func currentActionRequiresConcreteValidation(input map[string]any) bool {
	if strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state")) == convergenceStateNeedValidation {
		return true
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	switch nextAction {
	case runtimemodel.NextActionTypeValidate, "validation", "produce_validation_evidence":
		return true
	default:
		return false
	}
}

func compactCurrentActionDiagnostic(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"failureClass",
		"failureSignature",
		"reason",
		"violatedRule",
		"violationType",
		"correctionMode",
		"requiredOutputShape",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = cloneStepInputValue(value)
		}
	}
	if correction := mapFromValue(input["correctionContract"]); len(correction) > 0 {
		ret["correctionContract"] = cloneStepPayload(correction)
	}
	return ret
}

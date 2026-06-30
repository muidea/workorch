package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	currentActionContractKey    = "currentActionContract"
	currentActionContractSchema = "workorch.current_action_contract.v1"
)

func attachCurrentActionContractToInput(run *runModel.Run, stepID common.StepID, input map[string]any) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
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

func currentActionNextActionFromInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, source := range []map[string]any{
		mapFromValue(input[convergenceAuthorityKey]),
		mapFromValue(input[gapBaselineKey]),
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
	state := strings.TrimSpace(stringFromMap(contract, "state"))
	switch state {
	case convergenceStateNeedEvidence:
		if strings.TrimSpace(stringFromMap(contract, "runtimeStageMode")) == "target_snapshot_acquisition" {
			if parent := strings.TrimSpace(stringFromMap(mapFromValue(contract["parentNextAction"]), "kind")); parent != "" {
				return map[string]any{"kind": parent, "reason": "current convergence contract carries parent action for target snapshot evidence"}
			}
			if strings.TrimSpace(stringFromMap(mapFromValue(contract["prerequisite"]), "kind")) == "acquire_target_snapshot" {
				return map[string]any{"kind": "state_change", "reason": "state_change requires current target snapshot prerequisite"}
			}
			return map[string]any{"kind": "acquire_target_snapshot", "reason": "current convergence contract requires target snapshot evidence before state change"}
		}
		return map[string]any{"kind": "acquire_evidence", "reason": "current convergence contract requires bounded evidence before continuing"}
	case convergenceStateNeedStateChange:
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
	switch state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state")); state {
	case convergenceStateNeedGovernance:
		return nil
	case convergenceStateNeedAssessment:
		return assessmentRequiredOutputForAuthorityFocus(dispatchCoverageFocusFromAuthority(input, dispatchAuthorityCurrentFocus(input)))
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

func normalizeCurrentAuthorityRequiredOutput(input map[string]any, required map[string]any) map[string]any {
	if len(required) == 0 {
		return nil
	}
	authorityFocus := dispatchAuthorityCurrentFocus(input)
	if len(authorityFocus) == 0 {
		return cloneStepPayload(required)
	}
	return normalizeRequiredOutputFocusBinding(required, dispatchCoverageFocusFromAuthority(input, authorityFocus))
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

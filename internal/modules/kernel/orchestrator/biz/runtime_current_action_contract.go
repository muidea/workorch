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
	nextAction := mapFromValue(mapFromValue(input[convergenceAuthorityKey])["nextAction"])
	if len(nextAction) == 0 {
		nextAction = mapFromValue(mapFromValue(input[gapBaselineKey])["nextAction"])
	}
	if len(nextAction) == 0 {
		if kind := strings.TrimSpace(convergenceNextActionKind(run, input)); kind != "" {
			nextAction = map[string]any{"kind": kind}
		}
	}
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

func activeRequiredOutputForCurrentAction(input map[string]any) map[string]any {
	if required := mapFromValue(input["requiredOutput"]); len(required) > 0 {
		return required
	}
	if required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]); len(required) > 0 {
		return required
	}
	if required := mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]); len(required) > 0 {
		return required
	}
	return nil
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

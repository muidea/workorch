package orchestrator

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateRuntimeValidationStepInputBeforeRuntime(kind string, input map[string]any) error {
	if !runtimeStepKindCanProduceValidationEvidence(kind) || len(input) == 0 {
		return nil
	}
	if err := validateConvergenceDispatchFocusAlignment(input); err != nil {
		return err
	}
	if runtimeStepInputRequiresStateChangingDispatch(kind, input) {
		return nil
	}
	if !runtimeStepInputIsValidationContract(kind, input) {
		return nil
	}
	required := activeRequiredOutputForCurrentAction(input)
	if len(required) == 0 {
		return fmt.Errorf("%w: validation runtime dispatch requires current requiredOutput", common.ErrInvalidArgument)
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state"))
	if runtimeStepInputCurrentActionIsValidation(nextAction, state) {
		if requiredOutputRequiresStateChanging(required) || requiredOutputAcceptsKind(required, runtimemodel.RuntimeBlockKindPatchApply) {
			return fmt.Errorf("%w: validation runtime dispatch requires validation-only requiredOutput", common.ErrInvalidArgument)
		}
		if strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveStateChangingOutput || boolValue(input["stateChangingOutputRequired"]) {
			return fmt.Errorf("%w: validation runtime dispatch cannot keep state-changing repair objective", common.ErrInvalidArgument)
		}
	}
	if err := validateRequiredOutputFocusAlignment(dispatchAuthorityCurrentFocus(input), "requiredOutput", required); err != nil {
		return err
	}
	if err := validateRuntimeValidationEvidenceInputContract(kind, input); err != nil {
		return err
	}
	if strings.TrimSpace(kind) == runtimemodel.StepKindShellCommand && input["timeoutMillis"] == nil {
		return fmt.Errorf("%w: validation shell dispatch requires explicit timeoutMillis", common.ErrInvalidArgument)
	}
	return nil
}

func runtimeStepKindCanProduceValidationEvidence(kind string) bool {
	switch strings.TrimSpace(kind) {
	case runtimemodel.StepKindShellCommand, runtimemodel.StepKindMCPToolCall, managedValidationSessionNodeKind:
		return true
	default:
		return false
	}
}

func runtimeStepInputIsValidationContract(kind string, input map[string]any) bool {
	if runtimeStepInputRequiresStateChangingDispatch(kind, input) {
		return false
	}
	if len(mapFromValue(input["validationEvidence"])) > 0 {
		return true
	}
	if strings.TrimSpace(kind) == managedValidationSessionNodeKind {
		return true
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state"))
	if runtimeStepInputCurrentActionIsValidation(nextAction, state) {
		return true
	}
	return requiredOutputValidationOnly(activeRequiredOutputForCurrentAction(input))
}

func runtimeStepInputRequiresStateChangingDispatch(kind string, input map[string]any) bool {
	switch strings.TrimSpace(kind) {
	case runtimemodel.StepKindShellCommand, runtimemodel.StepKindMCPToolCall:
	default:
		return false
	}
	if boolValue(input["stateChanging"]) {
		return true
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state"))
	return state == convergenceStateNeedStateChange ||
		nextAction == runtimemodel.NextActionTypeStateChange ||
		nextAction == "produce_state_change"
}

func runtimeStepInputCurrentActionIsValidation(nextAction string, state string) bool {
	if state == convergenceStateNeedValidation {
		return true
	}
	switch nextAction {
	case runtimemodel.NextActionTypeValidate, "validation", "produce_validation_evidence":
		return true
	default:
		return false
	}
}

func validateRuntimeValidationEvidenceInputContract(kind string, input map[string]any) error {
	evidence := mapFromValue(input["validationEvidence"])
	if strings.TrimSpace(kind) == managedValidationSessionNodeKind {
		if len(evidence) == 0 {
			evidence = mapFromValue(mapFromValue(input["observation"])["validationEvidence"])
		}
	}
	if len(evidence) == 0 {
		return fmt.Errorf("%w: validation runtime dispatch requires validationEvidence", common.ErrInvalidArgument)
	}
	mode := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(evidence, "mode"),
		stringFromMap(input, "validationEvidenceMode"),
		stringFromMap(input, "evidenceMode"),
	))
	if !validationEvidenceModeIsConcrete(mode) {
		return fmt.Errorf("%w: validation runtime dispatch requires concrete validationEvidence.mode", common.ErrInvalidArgument)
	}
	observationType := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(evidence, "observationType"),
		stringFromMap(evidence, "observation"),
		stringFromMap(input, "validationObservationType"),
	))
	if !validationEvidenceObservationTypeIsConcrete(observationType) {
		return fmt.Errorf("%w: validation runtime dispatch requires concrete validationEvidence.observationType", common.ErrInvalidArgument)
	}
	if mode == "controlled_substitute" && !validationEvidenceHasControlledSubstitute(evidence) {
		return fmt.Errorf("%w: controlled substitute validation dispatch requires validationEvidence.substituteFor", common.ErrInvalidArgument)
	}
	evidenceInput := cloneStepPayload(input)
	evidenceInput["validationEvidence"] = evidence
	if len(runtimeValidationEvidenceBindingRefs(evidenceInput, evidence)) == 0 {
		return fmt.Errorf("%w: validation runtime dispatch requires validationEvidence targetRefs or coverageWorkItemIds", common.ErrInvalidArgument)
	}
	if len(nodeValidationEvidenceObservedClaims(runtimemodel.BlockExecutionNode{Input: evidenceInput})) == 0 {
		return fmt.Errorf("%w: validation runtime dispatch requires validationEvidence.observedClaims", common.ErrInvalidArgument)
	}
	return nil
}

func runtimeValidationEvidenceBindingRefs(input map[string]any, evidence map[string]any) []string {
	var refs []string
	refs = append(refs, nodeValidationEvidenceTargetRefs(runtimemodel.BlockExecutionNode{Input: input})...)
	refs = append(refs, stringSliceFromValue(evidence["coverageWorkItemIds"])...)
	refs = append(refs, stringSliceFromValue(evidence["coverageWorkItemIDs"])...)
	if id := strings.TrimSpace(stringFromMap(evidence, "coverageWorkItemId")); id != "" {
		refs = append(refs, id)
	}
	return compactUniqueStrings(refs)
}

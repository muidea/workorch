package orchestrator

import (
	"strings"
	"testing"

	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidateRuntimeValidationStepInputBeforeRuntimeRejectsMissingTimeout(t *testing.T) {
	input := validationDispatchInputFixture()
	delete(input, "timeoutMillis")

	err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input)
	if err == nil || !strings.Contains(err.Error(), "timeoutMillis") {
		t.Fatalf("error = %v, want explicit timeoutMillis rejection", err)
	}
}

func TestValidateRuntimeValidationStepInputBeforeRuntimeRejectsMissingRequiredOutput(t *testing.T) {
	input := validationDispatchInputFixture()
	delete(input, "requiredOutput")
	delete(input, "repairActionPackage")
	delete(input, currentActionContractKey)
	delete(input, stepInputKeyStagePacket)
	delete(input, "convergenceContract")

	err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input)
	if err == nil || !strings.Contains(err.Error(), "requiredOutput") {
		t.Fatalf("error = %v, want requiredOutput rejection", err)
	}
}

func TestValidateRuntimeValidationStepInputBeforeRuntimeRejectsFocusDrift(t *testing.T) {
	input := validationDispatchInputFixture()
	input["acceptance"] = map[string]any{
		"currentFocus": map[string]any{
			"id":                  "focus-cwi-other",
			"coverageWorkItemIds": []string{"cwi-other"},
		},
	}

	err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input)
	if err == nil || !strings.Contains(err.Error(), "focus mismatch") {
		t.Fatalf("error = %v, want focus mismatch rejection", err)
	}
}

func TestValidateRuntimeValidationStepInputBeforeRuntimeRejectsStateChangingObjective(t *testing.T) {
	input := validationDispatchInputFixture()
	input["repairObjective"] = repairObjectiveStateChangingOutput

	err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input)
	if err == nil || !strings.Contains(err.Error(), "state-changing repair objective") {
		t.Fatalf("error = %v, want state-changing objective rejection", err)
	}
}

func TestValidateRuntimeValidationStepInputBeforeRuntimeAcceptsBoundedValidation(t *testing.T) {
	input := validationDispatchInputFixture()

	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime: %v", err)
	}
}

func validationDispatchInputFixture() map[string]any {
	focus := map[string]any{
		"id":                  "focus-cwi-001",
		"coverageWorkItemIds": []string{"cwi-001"},
		"targetRefs":          []string{"cwi-001", "deliverable:source"},
	}
	required := map[string]any{
		"acceptedKinds":                            []string{runtimemodel.StepKindShellCommand, runtimemodel.StepKindMCPToolCall, managedValidationSessionNodeKind},
		"forbiddenKinds":                           []string{runtimemodel.RuntimeBlockKindPatchApply},
		"mustProduceConcreteValidationEvidence":    true,
		"mustDeclareValidationEvidenceMode":        true,
		"mustDeclareValidationEvidenceObservation": true,
		"coverageWorkItemIds":                      []string{"cwi-001"},
		"targetRefs":                               []string{"cwi-001", "deliverable:source"},
		"structuredTargetRefs":                     []string{"cwi-001", "deliverable:source"},
	}
	return map[string]any{
		"command":       "sh",
		"args":          []string{"-c", "true"},
		"timeoutMillis": int64(5000),
		"convergenceAuthority": map[string]any{
			"currentFocus": focus,
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedValidation,
		},
		coverageFocusKey: focus,
		"acceptance": map[string]any{
			"currentFocus": focus,
		},
		"requiredOutput": required,
		"repairActionPackage": map[string]any{
			"requiredOutput": required,
		},
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": required,
		},
		currentActionContractKey: map[string]any{
			"currentFocus":   focus,
			"requiredOutput": required,
			"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			"schemaVersion":  currentActionContractSchema,
		},
		"validationEvidence": map[string]any{
			"coverageWorkItemIds":  []string{"cwi-001"},
			"mode":                 "runtime_behavior",
			"observationType":      "runtime_observation",
			"observedClaims":       []string{"bounded validation observed"},
			"targetRefs":           []string{"cwi-001", "deliverable:source"},
			"structuredTargetRefs": []string{"cwi-001", "deliverable:source"},
		},
	}
}

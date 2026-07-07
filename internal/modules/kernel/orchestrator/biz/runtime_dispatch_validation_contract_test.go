package orchestrator

import (
	"errors"
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

func TestNormalizeRuntimeValidationStepInputBeforeRuntimeAddsShellTimeout(t *testing.T) {
	input := validationDispatchInputFixture()
	delete(input, "timeoutMillis")

	normalized, changed := normalizeRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input)
	if !changed {
		t.Fatal("changed = false, want validation shell timeout normalization")
	}
	if got := intFromAny(normalized["timeoutMillis"]); got != managedValidationTimeoutMillis {
		t.Fatalf("timeoutMillis = %d, want %d", got, managedValidationTimeoutMillis)
	}
	if _, ok := input["timeoutMillis"]; ok {
		t.Fatalf("input was mutated: %+v", input)
	}
	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, normalized); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime normalized input: %v", err)
	}
}

func TestNormalizeRuntimeValidationStepInputBeforeRuntimeSkipsStateChangingShell(t *testing.T) {
	input := validationDispatchInputFixture()
	delete(input, "timeoutMillis")
	input["stateChanging"] = true
	input["stateChangingOutputRequired"] = true
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["convergenceContract"] = map[string]any{"state": convergenceStateNeedStateChange}

	normalized, changed := normalizeRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input)
	if changed {
		t.Fatalf("changed = true, want state-changing shell unchanged: %+v", normalized)
	}
	if _, ok := normalized["timeoutMillis"]; ok {
		t.Fatalf("timeoutMillis added to state-changing shell: %+v", normalized)
	}
}

func TestValidateRuntimeStepKindRequiredOutputBeforeRuntimeRejectsUnacceptedKind(t *testing.T) {
	input := validationDispatchInputFixture()
	required := mapFromValue(input["requiredOutput"])
	required["acceptedKinds"] = []string{runtimemodel.StepKindShellCommand, managedValidationSessionNodeKind}
	input["requiredOutput"] = required
	input["repairActionPackage"] = map[string]any{"requiredOutput": required}
	input[stepInputKeyStagePacket] = map[string]any{"requiredOutput": required}
	input["runtimeCapabilities"] = map[string]any{"mcpServers": []map[string]any{{"id": "semantic-mcp"}}}
	delete(input, "convergenceContract")
	input[currentActionContractKey] = map[string]any{
		"currentFocus":   mapFromValue(input[coverageFocusKey]),
		"requiredOutput": required,
		"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		"schemaVersion":  currentActionContractSchema,
	}

	err := validateRuntimeStepKindRequiredOutputBeforeRuntime(runtimemodel.StepKindMCPToolCall, input)
	if err == nil || !strings.Contains(err.Error(), "not accepted") {
		t.Fatalf("error = %v, want unaccepted runtime kind rejection", err)
	}
	var structured runtimeContractViolationError
	if !errors.As(err, &structured) {
		t.Fatalf("error = %T %[1]v, want runtimeContractViolationError", err)
	}
	if structured.violationType != contractViolationValidationRequiredOutputCapabilityMismatch ||
		structured.rule != contractViolationRuleValidationRequiredOutputCapabilityMismatch ||
		structured.recoverability != runtimemodel.ContractViolationRecoverabilityRebuildValidationRequiredOutput {
		t.Fatalf("structured = %+v, want validation requiredOutput authority rebuild", structured)
	}
}

func TestValidateRuntimeStepKindRequiredOutputBeforeRuntimeRejectsForbiddenKind(t *testing.T) {
	input := validationDispatchInputFixture()
	required := mapFromValue(input["requiredOutput"])
	required["acceptedKinds"] = []string{runtimemodel.StepKindShellCommand, runtimemodel.StepKindMCPToolCall}
	required["forbiddenKinds"] = []string{runtimemodel.StepKindMCPToolCall}
	input["requiredOutput"] = required
	input["repairActionPackage"] = map[string]any{"requiredOutput": required}
	input[stepInputKeyStagePacket] = map[string]any{"requiredOutput": required}
	input[currentActionContractKey] = map[string]any{
		"currentFocus":   mapFromValue(input[coverageFocusKey]),
		"requiredOutput": required,
		"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		"schemaVersion":  currentActionContractSchema,
	}

	err := validateRuntimeStepKindRequiredOutputBeforeRuntime(runtimemodel.StepKindMCPToolCall, input)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %v, want forbidden runtime kind rejection", err)
	}
}

func TestValidateRuntimeStepKindRequiredOutputBeforeRuntimeSkipsLLMCarrierStep(t *testing.T) {
	input := validationDispatchInputFixture()
	required := mapFromValue(input["requiredOutput"])
	required["acceptedKinds"] = []string{runtimemodel.StepKindShellCommand}
	input["requiredOutput"] = required
	input["repairActionPackage"] = map[string]any{"requiredOutput": required}
	input[stepInputKeyStagePacket] = map[string]any{"requiredOutput": required}
	input[currentActionContractKey] = map[string]any{
		"currentFocus":   mapFromValue(input[coverageFocusKey]),
		"requiredOutput": required,
		"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		"schemaVersion":  currentActionContractSchema,
	}

	if err := validateRuntimeStepKindRequiredOutputBeforeRuntime(llmWorkspaceExecutionStepKind, input); err != nil {
		t.Fatalf("validateRuntimeStepKindRequiredOutputBeforeRuntime llm carrier: %v", err)
	}
}

func TestNormalizeConcreteValidationEvidenceNodeEnvelopeAddsShellTimeout(t *testing.T) {
	input := validationDispatchInputFixture()
	delete(input, "timeoutMillis")
	node := runtimemodel.BlockExecutionNode{
		Kind:  runtimemodel.StepKindShellCommand,
		Input: input,
	}
	envelope := concreteValidationEvidenceEnvelope{
		FocusID:        "focus-cwi-001",
		WorkItemIDs:    []string{"cwi-001"},
		TargetRefs:     []string{"cwi-001", "deliverable:source"},
		ObservedClaims: []string{"bounded validation observed"},
	}

	normalized, changed := normalizeConcreteValidationEvidenceNodeEnvelope(node, envelope)
	if !changed {
		t.Fatalf("changed = false, want timeout normalization")
	}
	if got := intFromAny(normalized.Input["timeoutMillis"]); got != managedValidationTimeoutMillis {
		t.Fatalf("timeoutMillis = %d, want %d", got, managedValidationTimeoutMillis)
	}
	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, normalized.Input); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime normalized input: %v", err)
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

func TestValidateRuntimeValidationStepInputBeforeRuntimeSkipsStateChangingShell(t *testing.T) {
	input := validationDispatchInputFixture()
	required := stateChangingRepairRequiredOutput()
	delete(input, "timeoutMillis")
	input["stateChanging"] = true
	input["stateChangingOutputRequired"] = true
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["convergenceAuthority"] = map[string]any{
		"currentFocus": mapFromValue(input[coverageFocusKey]),
		"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
	}
	input["convergenceContract"] = map[string]any{
		"state": convergenceStateNeedStateChange,
	}
	input["requiredOutput"] = required
	input["repairActionPackage"] = map[string]any{
		"requiredOutput": required,
	}
	input[stepInputKeyStagePacket] = map[string]any{
		"requiredOutput": required,
	}
	input[currentActionContractKey] = map[string]any{
		"currentFocus":   mapFromValue(input[coverageFocusKey]),
		"requiredOutput": required,
		"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
		"schemaVersion":  currentActionContractSchema,
	}

	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime: %v", err)
	}
}

func TestValidateRuntimeValidationStepInputBeforeRuntimeAcceptsBoundedValidation(t *testing.T) {
	input := validationDispatchInputFixture()

	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime: %v", err)
	}
}

func TestConcreteValidationRequiredOutputShellExampleMatchesDispatchGate(t *testing.T) {
	focus := map[string]any{
		"id":                   "focus-cwi-001",
		"coverageWorkItemIds":  []string{"cwi-001"},
		"targetRefs":           []string{"cwi-001", "deliverable:source"},
		"structuredTargetRefs": []string{"cwi-001", "deliverable:source"},
	}
	required := concreteValidationEvidenceRequiredOutputWithScratch(
		map[string]any{validationScratchDirKey: "/tmp/workorch-validation-test/example"},
		map[string]any{},
		nil,
	)
	applyValidationEvidenceExampleFocus(required, focus, []string{"cwi-001"})
	example := mapFromValue(required["validationEvidenceNodeExample"])
	input := mapFromValue(example["input"])
	input["convergenceAuthority"] = map[string]any{
		"currentFocus": focus,
		"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeValidate},
	}
	input["convergenceContract"] = map[string]any{
		"state": convergenceStateNeedValidation,
	}
	input[coverageFocusKey] = focus
	input["acceptance"] = map[string]any{"currentFocus": focus}
	input["requiredOutput"] = required
	input["repairActionPackage"] = map[string]any{"requiredOutput": required}
	input[stepInputKeyStagePacket] = map[string]any{"requiredOutput": required}
	input[currentActionContractKey] = map[string]any{
		"currentFocus":   focus,
		"requiredOutput": required,
		"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		"schemaVersion":  currentActionContractSchema,
	}

	if err := validateRuntimeValidationStepInputBeforeRuntime(runtimemodel.StepKindShellCommand, input); err != nil {
		t.Fatalf("validateRuntimeValidationStepInputBeforeRuntime example: %v", err)
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

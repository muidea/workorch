package orchestrator

import (
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAttachRuntimeFailureDiagnosticsPromotesValidationDiagnostic(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-validation-diagnostic/validate",
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Input: map[string]any{
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "dependency_resolution",
				"targetRefs":      []string{"dependency-contract"},
			},
		},
		Output: map[string]any{
			"status":   "failed",
			"command":  "validator",
			"exitCode": 2,
			"stderr":   "dependency contract mismatch",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("exit status 2"))
	attachRuntimeFailureClassification(step.Output, classification)
	attachRuntimeFailureDiagnostics(step, classification, errors.New("exit status 2"))

	diagnostic := mapFromValue(step.Output["validationFailureDiagnostic"])
	if diagnostic["class"] != runtimemodel.ValidationDiagnosticDependencyMismatch {
		t.Fatalf("validationFailureDiagnostic = %+v, want dependency mismatch", diagnostic)
	}
	if diagnostic["recommendedEvidenceTargetType"] != runtimemodel.ValidationEvidenceTargetDependencyContract {
		t.Fatalf("validationFailureDiagnostic = %+v, want dependency target type", diagnostic)
	}
	last := mapFromValue(step.Output["lastFailureDiagnostics"])
	if nested := mapFromValue(last["validationFailureDiagnostic"]); nested["class"] != runtimemodel.ValidationDiagnosticDependencyMismatch {
		t.Fatalf("lastFailureDiagnostics = %+v, want nested validation diagnostic", last)
	}
}

func TestRepairActionPackageCarriesValidationFailureDiagnostic(t *testing.T) {
	input := map[string]any{
		failureOutputKeyClass:     failureClassShellCommand,
		failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|exitCode=2",
		"failedStepOutput": map[string]any{
			"status":   "failed",
			"command":  "validator",
			"exitCode": 2,
			"validationFailureDiagnostic": map[string]any{
				"schemaVersion":                 runtimemodel.ValidationFailureDiagnosticSchemaVersion,
				"class":                         runtimemodel.ValidationDiagnosticToolchainMismatch,
				"recommendedEvidenceTargetType": runtimemodel.ValidationEvidenceTargetToolchainContract,
				"targetRefs":                    []string{"toolchain-contract"},
			},
		},
	}
	pkg := repairActionPackageFromRepairInput(input)
	diagnostic := mapFromValue(pkg["validationFailureDiagnostic"])
	if diagnostic["class"] != runtimemodel.ValidationDiagnosticToolchainMismatch {
		t.Fatalf("repairActionPackage.validationFailureDiagnostic = %+v, want toolchain mismatch", diagnostic)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustAddressValidationDiagnostic"] != true ||
		required["recommendedEvidenceTargetType"] != runtimemodel.ValidationEvidenceTargetToolchainContract {
		t.Fatalf("requiredOutput = %+v, want validation diagnostic target contract", required)
	}
}

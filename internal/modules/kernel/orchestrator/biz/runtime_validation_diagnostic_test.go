package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestRepairActionPackageAddsValidationScratchDirForConcreteValidationEvidence(t *testing.T) {
	input := map[string]any{
		"workspaceId":  "ws-scratch-contract",
		"runId":        "run-scratch-contract",
		"failedStepId": "run-scratch-contract/001-verify",
		"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
			"targetRef": "neutral-target",
			"reason":    "state change already present; validation evidence remains required",
		}},
	}

	pkg := repairActionPackageFromRepairInput(input)
	required := mapFromValue(pkg["requiredOutput"])
	scratch := stringFromMap(required, validationScratchDirKey)
	if scratch == "" {
		t.Fatalf("requiredOutput = %+v, want validationScratchDir", required)
	}
	if stringFromMap(input, validationScratchDirKey) != scratch || stringFromMap(pkg, validationScratchDirKey) != scratch {
		t.Fatalf("scratch contract not mirrored input=%q pkg=%q required=%q", stringFromMap(input, validationScratchDirKey), stringFromMap(pkg, validationScratchDirKey), scratch)
	}
	if !strings.Contains(stringFromMap(required, "controlledSubstituteScratchRule"), validationScratchDirKey) {
		t.Fatalf("requiredOutput = %+v, want scratch rule", required)
	}
}

func TestEnsureRuntimeValidationScratchDirCreatesOnlyControlledRoot(t *testing.T) {
	scratch := filepath.Join(os.TempDir(), "workorch-validation-scratch", "ws-scratch-runtime", "run", "step")
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(os.TempDir(), "workorch-validation-scratch", "ws-scratch-runtime"))
	})
	input := map[string]any{
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				validationScratchDirKey: scratch,
			},
		},
	}
	got, err := ensureRuntimeValidationScratchDir(input)
	if err != nil {
		t.Fatalf("ensure runtime validation scratch: %v", err)
	}
	if got != scratch || stringFromMap(input, validationScratchDirKey) != scratch {
		t.Fatalf("scratch = %q input=%q, want %q", got, stringFromMap(input, validationScratchDirKey), scratch)
	}
	if info, err := os.Stat(scratch); err != nil || !info.IsDir() {
		t.Fatalf("scratch dir stat = %v info=%+v, want directory", err, info)
	}

	outside := filepath.Join(os.TempDir(), "workorch-validation-scratch-sibling", "ws", "run", "step")
	_, err = ensureRuntimeValidationScratchDir(map[string]any{
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				validationScratchDirKey: outside,
			},
		},
	})
	if err == nil {
		t.Fatal("ensure runtime validation scratch outside controlled root succeeded, want error")
	}
	if _, statErr := os.Stat(outside); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside scratch stat = %v, want not created", statErr)
	}
}

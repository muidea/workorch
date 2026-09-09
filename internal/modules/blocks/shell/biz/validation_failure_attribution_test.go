package shell

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidationFailureAttributionKeepsFactAndRepairBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, text, wantClass, wantCode string
		observed, truncated, timeout, missing     bool
	}{
		{"legacy", shellGoTestObservationProtocolRef, "--- FAIL: TestCheck", runtimecontract.ValidationDiagnosticTestFailed, "test_failed", false, false, false, false},
		{"observed", shellGoBehaviorObservationProtocolRef, "--- FAIL: TestCheck", runtimecontract.ValidationDiagnosticTestFailed, "test_failed", true, false, false, false},
		{"unobserved", shellGoBehaviorObservationProtocolRef, "--- FAIL: TestCheck", runtimecontract.ValidationDiagnosticTestFailed, "validation_test_failure_unattributed", false, false, false, false},
		{"entrypoint", shellGoEntrypointObservationProtocolRef, "--- FAIL: TestCheck", runtimecontract.ValidationDiagnosticTestFailed, "validation_test_failure_unattributed", false, false, false, false},
		{"truncated", shellGoEntrypointObservationProtocolRef, "--- FAIL: TestCheck", runtimecontract.ValidationDiagnosticTestFailed, "validation_test_failure_unattributed", true, true, false, false},
		{"timeout", shellGoEntrypointObservationProtocolRef, "--- FAIL: TestCheck", runtimecontract.ValidationDiagnosticTimeout, "shell_exit_nonzero", false, false, true, false},
		{"missing environment", shellGoEntrypointObservationProtocolRef, "--- FAIL: TestCheck", runtimecontract.ValidationDiagnosticEnvironmentMissing, "command_not_found", false, false, false, true},
		{"dependency", shellGoEntrypointObservationProtocolRef, "no required module provides package example.invalid/pkg", runtimecontract.ValidationDiagnosticDependencyMismatch, "shell_exit_nonzero", false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := RuntimeCommandRequest{
				ManagedValidation:           runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
				ValidationScopeTargetRefs:   []string{"implementation.go", "fixture_test.go"},
				ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", NativeCheckRef: "TestCheck", ObservationProtocolRef: tc.protocol}},
			}
			output := RuntimeCommandResult{Command: "go", Args: []string{"test", "./..."}, ExitCode: 1, FailureCode: "shell_exit_nonzero", FailureReason: "shell_exit_nonzero", Stdout: tc.text, OutputTruncated: tc.truncated, CapabilityMissing: tc.missing}
			if tc.observed {
				output.workspaceBehaviorTargetRefs = []string{"implementation.go"}
			}
			attachShellValidationFailureDiagnostic(&output, request, "go", 1, tc.timeout, false, errors.New("exit status 1"))
			got := output.ValidationFailureDiagnostic
			if got == nil || got.Class != tc.wantClass || got.OwnerCode != tc.wantCode {
				t.Fatalf("diagnostic=%+v, want %s/%s", got, tc.wantClass, tc.wantCode)
			}
			if got.ExitCode != 1 || got.StdoutPreview != tc.text || got.InputRepair != nil || !reflect.DeepEqual(got.InputAddition, runtimecontract.RuntimeValidationInputAddition{}) {
				t.Fatalf("lost execution facts or invented input authorization: %+v", got)
			}
			if tc.wantCode == "validation_test_failure_unattributed" && (len(got.TargetRefs) != 0 || got.RecommendedEvidenceTargetType != runtimecontract.ValidationEvidenceTargetValidationOutput || !strings.Contains(got.Reason, "neither an implementation defect nor a validation-input defect")) {
				t.Fatalf("unattributed failure promoted to repair authority: %+v", got)
			}
			if tc.name == "observed" && !reflect.DeepEqual(got.TargetRefs, []string{"implementation.go"}) {
				t.Fatalf("lost implementation observation: %+v", got)
			}
			attachShellValidationFailureDiagnostic(&output, request, "go", 1, tc.timeout, false, errors.New("exit status 1"))
			if !reflect.DeepEqual(got, output.ValidationFailureDiagnostic) {
				t.Fatal("repeated observation changed diagnostic")
			}
		})
	}
}

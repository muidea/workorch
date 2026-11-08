package shell

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellFailureTargetCapacityMatchesProducerAndTypedConsumer(t *testing.T) {
	for _, n := range []int{8, 9, 16, 17} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			refs := make([]string, n)
			for i := range refs {
				refs[i] = fmt.Sprintf("internal/part-%02d.go", i)
			}
			output := RuntimeCommandResult{Command: "go", Args: []string{"test", "./..."}, workspaceBehaviorTargetRefs: refs, ExitCode: 1}
			request := RuntimeCommandRequest{ValidationEvidence: rc.RuntimeValidationEvidence{DiagnosticClass: rc.ValidationDiagnosticTestFailed}, ValidationScopeTargetRefs: refs, ManagedValidation: rc.RuntimeManagedValidation{RuntimeManaged: true, Kind: rc.RuntimeBlockKindManagedValidationSession}}
			// Real project adapter selects independently observed implementation behavior.
			request.ValidationCheckAttestations = []rc.RuntimeValidationCheckAttestationBinding{{ObservationProtocolRef: shellGoBehaviorObservationProtocolRef}}
			attachShellValidationFailureDiagnostic(&output, request, "go", 1, false, false, fmt.Errorf("exit status 1"))
			if output.ValidationFailureDiagnostic == nil {
				t.Fatal("producer lost diagnostic")
			}
			got, valid, rejection := shellFailureTargetCandidateUnion(output, "implementation", refs)
			if n <= rc.RuntimeFailureTargetObservationMaxTargetRefs {
				if !valid || rejection != nil || !reflect.DeepEqual(got, refs) {
					t.Fatalf("complete producer union lost: refs=%v valid=%t rejection=%+v", got, valid, rejection)
				}
				o := rc.RuntimeValidationFailureObservation{WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: "validate", FailureSignature: "failure", TargetRole: "implementation", DiagnosticClass: rc.ValidationDiagnosticTestFailed, CandidateTargetRefs: got, ValidationScopeTargetRefs: refs, DiagnosticHash: rc.StableRuntimeValidationFailureObservationDiagnosticHash(*output.ValidationFailureDiagnostic, refs)}
				if err := o.Validate(); err != nil {
					t.Fatal(err)
				}
			} else if valid || len(got) != 0 || rejection == nil || rejection.Code != rc.RuntimeFailureTargetsOverCapacity || rejection.CandidateCount != n || rejection.Validate() != nil {
				t.Fatalf("overflow not explicit: valid=%t refs=%v rejection=%+v", valid, got, rejection)
			}
		})
	}
}

func TestShellCompilerTargetOverflowRetainsWholeRejectedCount(t *testing.T) {
	var output strings.Builder
	var refs []string
	for i := 0; i < 19; i++ {
		ref := fmt.Sprintf("part-%02d.go", i)
		refs = append(refs, ref)
		fmt.Fprintf(&output, "%s:1:1: undefined: missing\n", ref)
	}
	result := RuntimeCommandResult{Command: "go", Args: []string{"build", "./..."}, Stderr: output.String(), ExitCode: 1}
	request := RuntimeCommandRequest{ValidationScopeTargetRefs: refs, ManagedValidation: rc.RuntimeManagedValidation{RuntimeManaged: true, Kind: rc.RuntimeBlockKindManagedValidationSession}}
	attachShellValidationFailureDiagnostic(&result, request, "go", 1, false, false, fmt.Errorf("exit status 1"))
	if result.ValidationFailureDiagnostic == nil || len(result.ValidationFailureDiagnostic.TargetRefs) != 0 {
		t.Fatal("compiler overflow published a partial target surface")
	}
	got, valid, rejection := shellFailureTargetCandidateUnion(result, "implementation", refs)
	if valid || len(got) != 0 || rejection == nil || rejection.Code != rc.RuntimeFailureTargetsOverCapacity || rejection.CandidateCount != len(refs) || rejection.Validate() != nil {
		t.Fatalf("compiler overflow lost complete rejected count: valid=%t refs=%v rejection=%+v", valid, got, rejection)
	}
}

func TestShellBehaviorSurfaceExcludesAssertionLocationCapacity(t *testing.T) {
	var output strings.Builder
	refs := []string{"implementation.go"}
	for i := 0; i < 19; i++ {
		ref := fmt.Sprintf("assertion-%02d_test.go", i)
		refs = append(refs, ref)
		fmt.Fprintf(&output, "%s:1:1: assertion failed\n", ref)
	}
	result := RuntimeCommandResult{Command: "go", Args: []string{"test", "./..."}, Stderr: output.String(), ExitCode: 1, workspaceBehaviorTargetRefs: []string{"implementation.go"}}
	request := RuntimeCommandRequest{ValidationEvidence: rc.RuntimeValidationEvidence{DiagnosticClass: rc.ValidationDiagnosticTestFailed}, ValidationScopeTargetRefs: refs, ManagedValidation: rc.RuntimeManagedValidation{RuntimeManaged: true, Kind: rc.RuntimeBlockKindManagedValidationSession}, ValidationCheckAttestations: []rc.RuntimeValidationCheckAttestationBinding{{ObservationProtocolRef: shellGoBehaviorObservationProtocolRef}}}
	attachShellValidationFailureDiagnostic(&result, request, "go", 1, false, false, fmt.Errorf("exit status 1"))
	got, valid, rejection := shellFailureTargetCandidateUnion(result, "implementation", refs)
	if !valid || rejection != nil || !reflect.DeepEqual(got, []string{"implementation.go"}) {
		t.Fatalf("assertion locations replaced independently observed implementation behavior: refs=%v valid=%t rejection=%+v", got, valid, rejection)
	}
}

func TestShellFailureCandidateRejectsCompleteMarkerUnionAndScope(t *testing.T) {
	refs := make([]string, 16)
	for i := range refs {
		refs[i] = fmt.Sprintf("part-%02d.go", i)
	}
	for _, tt := range []struct {
		name   string
		marker []string
		scope  []string
		code   string
	}{
		{"union overflow", []string{"extra.go"}, append(append([]string(nil), refs...), "extra.go"), rc.RuntimeFailureTargetsOverCapacity},
		{"outside scope", []string{"outside.go"}, refs, rc.RuntimeFailureTargetsOutsideScope},
		{"marker overflow", append(append([]string(nil), refs...), "extra.go"), append(append([]string(nil), refs...), "extra.go"), rc.RuntimeFailureTargetsOverCapacity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			marker, _ := json.Marshal(shellFailureTargetRecord{SchemaVersion: shellFailureTargetMarkerSchema, TargetRole: "implementation", TargetRefs: tt.marker, DiagnosticClass: rc.ValidationDiagnosticTestFailed})
			output := RuntimeCommandResult{Stdout: shellFailureTargetMarkerPrefix + string(marker), ValidationFailureDiagnostic: &rc.ValidationFailureDiagnostic{Class: rc.ValidationDiagnosticTestFailed, TargetRefs: refs}}
			got, valid, issue := shellFailureTargetCandidateUnion(output, "implementation", tt.scope)
			if valid || len(got) != 0 || issue == nil || issue.Code != tt.code || issue.Validate() != nil {
				t.Fatalf("partial union/ambiguous rejection: refs=%v valid=%t issue=%+v", got, valid, issue)
			}
		})
	}
}

func TestShellFailureCandidateRetainsTypedRejectionWithoutLocalParserState(t *testing.T) {
	issue := &rc.RuntimeValidationFailureTargetRejection{Code: rc.RuntimeFailureTargetsOverCapacity, CandidateCount: 17, Limit: rc.RuntimeFailureTargetObservationMaxTargetRefs}
	// Owner DTO copies do not carry private parser state. The typed result must
	// itself preserve the complete rejected transaction, never resurrect a subset.
	output := RuntimeCommandResult{ValidationFailureDiagnostic: &rc.ValidationFailureDiagnostic{Class: rc.ValidationDiagnosticTestFailed, TargetRejection: issue}}
	refs, valid, got := shellFailureTargetCandidateUnion(output, "implementation", []string{"target"})
	if valid || len(refs) != 0 || !reflect.DeepEqual(got, issue) {
		t.Fatal("typed rejection was lost without the private parser flag")
	}
	got.CandidateCount++
	if issue.CandidateCount != 17 {
		t.Fatal("candidate boundary aliased original diagnostic")
	}
}

package workorchruntimecontract

import (
	"strings"
	"testing"
)

func TestRuntimeRepairSurfaceObservationIsFactOnlyAndBounded(t *testing.T) {
	observation := RuntimeValidationFailureObservation{
		WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: "validate",
		FailureSignature: "build-failure", TargetRole: "implementation", DiagnosticClass: ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"b.go", "a.go"}, ValidationScopeTargetRefs: []string{"a.go", "b.go", "test.go"}, DiagnosticHash: strings.Repeat("a", 64),
	}
	if err := observation.Validate(); err != nil {
		t.Fatalf("observation: %v", err)
	}
	confirmation := RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: []string{"a.go", "b.go"}, ObservedRevision: "workspace-v1"}
	if err := confirmation.Validate(); err != nil {
		t.Fatalf("confirmation: %v", err)
	}
	invalid := confirmation
	invalid.ConfirmedTargetRefs = []string{"test.go"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("confirmation accepted targets not proposed by validation fact")
	}
	invalid = confirmation
	invalid.Observation.CandidateTargetRefs = []string{"outside.go"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("observation accepted candidate outside sealed validation scope")
	}
}

package proof

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRepairSurfaceFactsFenceExactReplayConflictAndCausation(t *testing.T) {
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: "workspace-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", DispatchID: "dispatch-1", OperationID: "validate-1",
		FailureSignature: "failure-1", TargetRole: "implementation", DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("a", 64),
	}
	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: []string{"main.go"}, ObservedRevision: "tree-1",
	}
	record, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(confirmation)
	if err != nil {
		t.Fatalf("fixture confirmation: %v", err)
	}
	if !RepairSurfaceConfirmationExists([]runtimecontract.RuntimeRepairSurfaceConfirmationRecord{record}, confirmation) {
		t.Fatal("exact confirmation replay was not recognized")
	}
	conflict := confirmation.Clone()
	conflict.Disposition = runtimecontract.RuntimeRepairSurfaceRejected
	conflict.ConfirmedTargetRefs = nil
	conflict.Reason = "not a regular file"
	if !RepairSurfaceConfirmationIdentityConflict([]runtimecontract.RuntimeRepairSurfaceConfirmationRecord{record}, conflict) {
		t.Fatal("conflicting disposition for one owner observation was not rejected")
	}

	operations := []SealedValidationOperation{{
		DispatchID: "dispatch-1", AuthoritySnapshotID: "authority-1", OperationID: "validate-1", TargetRole: "implementation", TargetRefs: []string{"main.go"},
	}}
	if !ValidationFailureObservationMatchesSealedOperation(operations, observation) {
		t.Fatal("exact completed validation operation did not bind the observation")
	}
	stale := observation.Clone()
	stale.AuthoritySnapshotID = "authority-old"
	if ValidationFailureObservationMatchesSealedOperation(operations, stale) {
		t.Fatal("stale authority observation matched the completed operation")
	}
	forged := observation.Clone()
	forged.ValidationScopeTargetRefs = []string{"other.go"}
	if ValidationFailureObservationMatchesSealedOperation(operations, forged) {
		t.Fatal("different validation scope matched the completed operation")
	}
	forged = observation.Clone()
	forged.TargetRole = "prospective-implementation"
	if ValidationFailureObservationMatchesSealedOperation(operations, forged) {
		t.Fatal("different sealed focus target role matched the completed operation")
	}
}

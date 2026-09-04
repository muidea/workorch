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
		RequestID: "repair-request-1", IssuedAtStateRevision: 7, ExpectedSubjectRevision: "tree-1",
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
}

func TestValidationFailureObservationMatchesSealedOperationUsesSealedValidationScope(t *testing.T) {
	operation := SealedValidationOperation{
		DispatchID:          "dispatch-1",
		AuthoritySnapshotID: "authority-1",
		OperationID:         "validate-1",
		TargetRole:          "implementation",
		OperationTargetRefs: []string{"internal/lsp/server_test.go"},
		RequireTargets:      true,
		TargetAuthorization: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:               runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerScopeTargetRefs: []string{"internal/lsp/server.go", "internal/lsp/server_test.go"},
		},
	}
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID:               "workspace-1",
		RunID:                     "run-1",
		AuthoritySnapshotID:       "authority-1",
		DispatchID:                "dispatch-1",
		OperationID:               "validate-1",
		FailureSignature:          "failure-1",
		TargetRole:                "implementation",
		DiagnosticClass:           runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs:       []string{"internal/lsp/server.go"},
		ValidationScopeTargetRefs: []string{"internal/lsp/server.go", "internal/lsp/server_test.go"},
		DiagnosticHash:            strings.Repeat("a", 64),
	}

	if !ValidationFailureObservationMatchesSealedOperation([]SealedValidationOperation{operation}, observation) {
		t.Fatal("diagnostic candidate inside the sealed validation scope must bind even when it differs from the validation entry target")
	}

	reordered := observation.Clone()
	reordered.ValidationScopeTargetRefs = []string{"internal/lsp/server_test.go", "internal/lsp/server.go"}
	if !ValidationFailureObservationMatchesSealedOperation([]SealedValidationOperation{operation}, reordered) {
		t.Fatal("equivalent normalized validation scope did not bind the completed operation")
	}
}

func TestValidationFailureObservationMatchesSealedOperationRejectsUnsealedOrInvalidFacts(t *testing.T) {
	operation := SealedValidationOperation{
		DispatchID:          "dispatch-1",
		AuthoritySnapshotID: "authority-1",
		OperationID:         "validate-1",
		TargetRole:          "implementation",
		OperationTargetRefs: []string{"internal/lsp/server_test.go"},
		RequireTargets:      true,
		TargetAuthorization: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:               runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerScopeTargetRefs: []string{"internal/lsp/server.go", "internal/lsp/server_test.go"},
		},
	}
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID:               "workspace-1",
		RunID:                     "run-1",
		AuthoritySnapshotID:       "authority-1",
		DispatchID:                "dispatch-1",
		OperationID:               "validate-1",
		FailureSignature:          "failure-1",
		TargetRole:                "implementation",
		DiagnosticClass:           runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs:       []string{"internal/lsp/server.go"},
		ValidationScopeTargetRefs: []string{"internal/lsp/server.go", "internal/lsp/server_test.go"},
		DiagnosticHash:            strings.Repeat("a", 64),
	}

	tests := []struct {
		name        string
		mutateOp    func(*SealedValidationOperation)
		mutateFacts func(*runtimecontract.RuntimeValidationFailureObservation)
	}{
		{
			name: "wrong dispatch",
			mutateFacts: func(value *runtimecontract.RuntimeValidationFailureObservation) {
				value.DispatchID = "dispatch-old"
			},
		},
		{
			name: "wrong authority snapshot",
			mutateFacts: func(value *runtimecontract.RuntimeValidationFailureObservation) {
				value.AuthoritySnapshotID = "authority-old"
			},
		},
		{
			name: "wrong operation",
			mutateFacts: func(value *runtimecontract.RuntimeValidationFailureObservation) {
				value.OperationID = "validate-other"
			},
		},
		{
			name: "wrong target role",
			mutateFacts: func(value *runtimecontract.RuntimeValidationFailureObservation) {
				value.TargetRole = "prospective-implementation"
			},
		},
		{
			name: "validation scope missing sealed target",
			mutateFacts: func(value *runtimecontract.RuntimeValidationFailureObservation) {
				value.CandidateTargetRefs = []string{"internal/lsp/server_test.go"}
				value.ValidationScopeTargetRefs = []string{"internal/lsp/server_test.go"}
			},
		},
		{
			name: "validation scope adds unsealed target",
			mutateFacts: func(value *runtimecontract.RuntimeValidationFailureObservation) {
				value.ValidationScopeTargetRefs = append(value.ValidationScopeTargetRefs, "internal/lsp/other.go")
			},
		},
		{
			name: "diagnostic candidate outside validation scope",
			mutateFacts: func(value *runtimecontract.RuntimeValidationFailureObservation) {
				value.CandidateTargetRefs = []string{"internal/lsp/other.go"}
			},
		},
		{
			name: "diagnostic candidate empty",
			mutateFacts: func(value *runtimecontract.RuntimeValidationFailureObservation) {
				value.CandidateTargetRefs = nil
			},
		},
		{
			name: "operation target outside sealed validation scope",
			mutateOp: func(value *SealedValidationOperation) {
				value.OperationTargetRefs = []string{"internal/lsp/other_test.go"}
			},
		},
		{
			name: "missing sealed validation scope",
			mutateOp: func(value *SealedValidationOperation) {
				value.TargetAuthorization.OwnerScopeTargetRefs = nil
				value.TargetAuthorization.OwnerSelectorRefs = []string{"workspace-validation-target"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateOperation := operation
			candidateOperation.OperationTargetRefs = append([]string(nil), operation.OperationTargetRefs...)
			candidateOperation.TargetAuthorization = operation.TargetAuthorization.Normalized()
			candidateObservation := observation.Clone()
			if test.mutateOp != nil {
				test.mutateOp(&candidateOperation)
			}
			if test.mutateFacts != nil {
				test.mutateFacts(&candidateObservation)
			}
			if ValidationFailureObservationMatchesSealedOperation([]SealedValidationOperation{candidateOperation}, candidateObservation) {
				t.Fatal("unsealed or invalid validation failure facts matched the completed operation")
			}
		})
	}

	if ValidationFailureObservationMatchesSealedOperation(nil, observation) {
		t.Fatal("observation matched without a completed sealed validation operation")
	}
}

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
	invalid = confirmation
	invalid.OwnerModuleID = RuntimeModuleMCP
	if err := invalid.Validate(); err == nil {
		t.Fatal("repair surface confirmation accepted a non-File owner")
	}
	overflow := observation
	for index := len(overflow.CandidateTargetRefs); index <= RuntimeValidationFailureObservationMaxCandidateTargetRefs; index++ {
		ref := string(rune('c'+index)) + ".go"
		overflow.CandidateTargetRefs = append(overflow.CandidateTargetRefs, ref)
		overflow.ValidationScopeTargetRefs = append(overflow.ValidationScopeTargetRefs, ref)
	}
	if err := overflow.Validate(); err == nil {
		t.Fatal("validation failure observation accepted an over-bounded candidate set")
	}
}

func TestRuntimeRepairSurfaceConfirmationRecordProvidesTypedReplayFence(t *testing.T) {
	observation := RuntimeValidationFailureObservation{
		WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: "validate",
		FailureSignature: "build-failure", TargetRole: "implementation", DiagnosticClass: ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("b", 64),
	}
	confirmed := RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: []string{"main.go"}, ObservedRevision: "tree-v1", Reason: "same observation"}
	record, err := NewRuntimeRepairSurfaceConfirmationRecord(confirmed)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("record validation: %v", err)
	}
	stale := confirmed
	stale.Disposition = RuntimeRepairSurfaceStale
	stale.ConfirmedTargetRefs = nil
	if StableRuntimeRepairSurfaceConfirmationHash(confirmed) == StableRuntimeRepairSurfaceConfirmationHash(stale) {
		t.Fatal("different dispositions share exact replay signature")
	}
	if StableRuntimeRepairSurfaceConfirmationIdentityHash(confirmed) != StableRuntimeRepairSurfaceConfirmationIdentityHash(stale) {
		t.Fatal("same owner observation does not share identity signature")
	}
	changedReason := confirmed
	changedReason.Reason = "different owner explanation"
	if StableRuntimeRepairSurfaceConfirmationIdentityHash(confirmed) != StableRuntimeRepairSurfaceConfirmationIdentityHash(changedReason) {
		t.Fatal("response reason changed the owner-observation identity")
	}
	if err := ValidateRuntimeRepairSurfaceConfirmationRecords([]RuntimeRepairSurfaceConfirmationRecord{record, record}); err == nil {
		t.Fatal("duplicate confirmation records were accepted")
	}
	mutated := record.Clone()
	mutated.Confirmation.Observation.CandidateTargetRefs[0] = "other.go"
	if record.Confirmation.Observation.CandidateTargetRefs[0] != "main.go" {
		t.Fatal("confirmation record clone shares nested owner fact")
	}
	mutated.Signature = StableRuntimeRepairSurfaceConfirmationHash(mutated.Confirmation)
	if err := mutated.Validate(); err == nil {
		t.Fatal("mutated record unexpectedly retained a valid identity signature")
	}
}

func TestRuntimeRepairSurfaceConfirmationSequenceEndsAtTerminalDisposition(t *testing.T) {
	observation := RuntimeValidationFailureObservation{
		WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: "validate",
		FailureSignature: "build-failure", TargetRole: "implementation", DiagnosticClass: ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("c", 64),
	}
	makeRecord := func(disposition RuntimeRepairSurfaceDisposition, revision string) RuntimeRepairSurfaceConfirmationRecord {
		value := RuntimeRepairSurfaceConfirmation{
			Observation: observation, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable",
			Disposition: disposition, ObservedRevision: revision,
		}
		if disposition == RuntimeRepairSurfaceConfirmed {
			value.ConfirmedTargetRefs = []string{"main.go"}
		}
		record, err := NewRuntimeRepairSurfaceConfirmationRecord(value)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	staleV1 := makeRecord(RuntimeRepairSurfaceStale, "tree-v1")
	staleV2 := makeRecord(RuntimeRepairSurfaceStale, "tree-v2")
	confirmedV3 := makeRecord(RuntimeRepairSurfaceConfirmed, "tree-v3")
	if err := ValidateRuntimeRepairSurfaceConfirmationRecords([]RuntimeRepairSurfaceConfirmationRecord{staleV1, staleV2, confirmedV3}); err != nil {
		t.Fatalf("stale refresh sequence with one terminal response: %v", err)
	}
	late := makeRecord(RuntimeRepairSurfaceRejected, "tree-v4")
	if err := ValidateRuntimeRepairSurfaceConfirmationRecords([]RuntimeRepairSurfaceConfirmationRecord{staleV1, confirmedV3, late}); err == nil || !strings.Contains(err.Error(), "follows terminal response") {
		t.Fatalf("post-terminal confirmation sequence error = %v", err)
	}
	driftedSubject := staleV2.Clone()
	driftedSubject.Confirmation.SubjectRef = "another_subject"
	driftedSubject.Signature = StableRuntimeRepairSurfaceConfirmationHash(driftedSubject.Confirmation)
	driftedSubject.IdentitySignature = StableRuntimeRepairSurfaceConfirmationIdentityHash(driftedSubject.Confirmation)
	if err := ValidateRuntimeRepairSurfaceConfirmationRecords([]RuntimeRepairSurfaceConfirmationRecord{staleV1, driftedSubject}); err == nil || !strings.Contains(err.Error(), "changes subject") {
		t.Fatalf("confirmation subject drift error = %v", err)
	}
}

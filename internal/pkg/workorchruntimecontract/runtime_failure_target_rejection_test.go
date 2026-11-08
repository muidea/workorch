package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidationFailureTargetRejectionPersistsWithoutPartialAuthority(t *testing.T) {
	d := ValidationFailureDiagnostic{Class: ValidationDiagnosticTestFailed, TargetRejection: &RuntimeValidationFailureTargetRejection{Code: RuntimeFailureTargetsOverCapacity, CandidateCount: 17, Limit: RuntimeFailureTargetObservationMaxTargetRefs}}
	if err := validateRuntimeValidationInputDiagnostic(d); err != nil {
		t.Fatal(err)
	}
	hash := StableRuntimeValidationFailureObservationDiagnosticHash(d, []string{"scope"})
	raw, _ := json.Marshal(d)
	var loaded ValidationFailureDiagnostic
	if err := json.Unmarshal(raw, &loaded); err != nil || !SameValidationFailureDiagnostic(&d, &loaded) || hash != StableRuntimeValidationFailureObservationDiagnosticHash(loaded, []string{"scope"}) {
		t.Fatal("rejection lost on reload", err)
	}
	copy := cloneValidationFailureDiagnosticRaw(d)
	copy.TargetRejection.CandidateCount++
	if d.TargetRejection.CandidateCount != 17 || hash == StableRuntimeValidationFailureObservationDiagnosticHash(copy, []string{"scope"}) {
		t.Fatal("rejection not isolated/hash-bound")
	}
	copy = d.Clone()
	copy.TargetRefs = []string{"partial.go"}
	if err := validateRuntimeValidationInputDiagnostic(copy); err == nil {
		t.Fatal("rejection admitted a partial target set")
	}
	copy = d.Clone()
	copy.TargetRejection.CandidateCount = 16
	if err := validateRuntimeValidationInputDiagnostic(copy); err == nil {
		t.Fatal("capacity reason admitted below limit")
	}
	copy = d.Clone()
	copy.TargetRejection.Code = "unknown"
	if err := validateRuntimeValidationInputDiagnostic(copy); err == nil {
		t.Fatal("unknown rejection accepted")
	}
	copy = d.Clone()
	copy.TargetRejection.Code, copy.TargetRejection.CandidateCount = RuntimeFailureTargetsOutsideScope, 0
	if err := validateRuntimeValidationInputDiagnostic(copy); err == nil {
		t.Fatal("scope rejection without observed candidates accepted")
	}
	o := RuntimeValidationFailureObservation{WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "a", DispatchID: "d", OperationID: "op", FailureSignature: "f", TargetRole: "implementation", DiagnosticClass: d.Class, CandidateTargetRefs: []string{"scope"}, ValidationScopeTargetRefs: []string{"scope"}, DiagnosticHash: strings.Repeat("a", 64)}
	if err := ValidateRuntimeValidationDiagnosticFailureObservation(nil, RuntimeValidationProofRequirementBinding{}, []string{"scope"}, &o, &d); err == nil {
		t.Fatal("rejected union gained repair authority")
	}
}

func TestValidationFailureTargetRejectionBindsReceiptAndFeedback(t *testing.T) {
	diagnostic := &ValidationFailureDiagnostic{Class: ValidationDiagnosticTestFailed,
		TargetRejection: &RuntimeValidationFailureTargetRejection{Code: RuntimeFailureTargetsOverCapacity, CandidateCount: 17, Limit: RuntimeFailureTargetObservationMaxTargetRefs}}
	receipt := RuntimeOperationReceipt{ExecutionKey: "execution", Fence: 1, OperationID: "validation",
		Feedback: &RuntimeFeedback{OriginalDiagnostic: &RuntimeOriginalDiagnostic{ValidationDiagnostic: diagnostic}}}
	receiptHash, feedbackHash := StableRuntimeOperationReceiptHash(receipt), StableRuntimeFeedbackHash(*receipt.Feedback)
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeOperationReceipt
	if err := json.Unmarshal(raw, &loaded); err != nil || StableRuntimeOperationReceiptHash(loaded) != receiptHash || StableRuntimeFeedbackHash(*loaded.Feedback) != feedbackHash {
		t.Fatal("receipt reload lost exact rejection", err)
	}
	cloned := loaded.Clone()
	cloned.Feedback.OriginalDiagnostic.ValidationDiagnostic.TargetRejection.CandidateCount++
	if diagnostic.TargetRejection.CandidateCount != 17 || loaded.Feedback.OriginalDiagnostic.ValidationDiagnostic.TargetRejection.CandidateCount != 17 {
		t.Fatal("receipt clone shared rejection storage")
	}
	if StableRuntimeOperationReceiptHash(cloned) == receiptHash || StableRuntimeFeedbackHash(*cloned.Feedback) == feedbackHash {
		t.Fatal("receipt/feedback hash omitted rejection count")
	}
	cloned = loaded.Clone()
	cloned.Feedback.OriginalDiagnostic.ValidationDiagnostic.TargetRejection = nil
	if StableRuntimeOperationReceiptHash(cloned) == receiptHash {
		t.Fatal("deleting rejection preserved receipt identity")
	}
}

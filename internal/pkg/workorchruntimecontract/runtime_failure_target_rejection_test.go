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

func TestValidationFailureLocationsSurviveSanitizationHashAndValidate(t *testing.T) {
	diagnostic := &ValidationFailureDiagnostic{
		Class:      ValidationDiagnosticBuildFailed,
		OwnerCode:  "source_compile_failed",
		TargetRefs: []string{"cmd/app/main.go"},
		FailureLocations: []RuntimeValidationFailureLocation{
			{TargetRef: "cmd/app/main.go", Line: 298, Column: 17, Message: "cannot use w as http.Header"},
		},
		StdoutPreview: "raw stdout must be cleared",
		StderrPreview: "raw stderr must be cleared",
	}
	sanitized := RuntimeOriginalDiagnosticFromFailureFact("blocks/shell", FailureFact{
		Class:                FailureClassBuildFailure,
		Code:                 "validation_project_stage_failed",
		Message:              "compile failed",
		Signature:            "sig-1",
		ValidationDiagnostic: diagnostic,
	})
	if sanitized == nil || sanitized.ValidationDiagnostic == nil {
		t.Fatal("expected sanitized original diagnostic")
	}
	if sanitized.ValidationDiagnostic.StdoutPreview != "" || sanitized.ValidationDiagnostic.StderrPreview != "" {
		t.Fatal("raw previews were not stripped")
	}
	if len(sanitized.ValidationDiagnostic.FailureLocations) != 1 || sanitized.ValidationDiagnostic.FailureLocations[0].Line != 298 || sanitized.ValidationDiagnostic.FailureLocations[0].Column != 17 {
		t.Fatalf("failure locations lost on sanitization: %+v", sanitized.ValidationDiagnostic.FailureLocations)
	}
	fb := validRuntimeFeedback()
	fb.OriginalDiagnostic = sanitized
	if err := fb.Validate(); err != nil {
		t.Fatalf("valid failure locations rejected: %v", err)
	}
	fbHash := StableRuntimeFeedbackHash(fb)
	fbChanged := cloneRuntimeFeedbackRaw(fb)
	fbChanged.OriginalDiagnostic.ValidationDiagnostic.FailureLocations[0].Line++
	if StableRuntimeFeedbackHash(fbChanged) == fbHash {
		t.Fatal("feedback hash ignored FailureLocations change")
	}
	overLimit := make([]RuntimeValidationFailureLocation, RuntimeValidationFailureLocationMaxCount+1)
	for i := range overLimit {
		overLimit[i] = RuntimeValidationFailureLocation{TargetRef: "a.go", Line: i + 1, Message: "err"}
	}
	if err := ValidateRuntimeValidationFailureLocations(overLimit); err == nil {
		t.Fatal("expected over-capacity failure locations to fail validation")
	}
	if err := ValidateRuntimeValidationFailureLocations([]RuntimeValidationFailureLocation{{TargetRef: "a.go", Line: 1, Message: "bad\x00msg"}}); err == nil {
		t.Fatal("expected NUL byte in failure location message to fail validation")
	}
}

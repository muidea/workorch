package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func validationAssertionFeedbackFixture() EvidenceFact {
	fact := validationFeedbackFixture()
	fact.Validation.FailureDetailProtocolRef = RuntimeValidationFailureDetailProtocolV2
	fact.Validation.CheckFailures[0].ComparisonHash = strings.Repeat("c", 64)
	fact.Validation.CheckFailures[0].ReportedAssertion = RuntimeValidationReportedAssertion{
		Code: "shape_mismatch", ExpectedShape: "object", ObservedShape: "array",
	}
	return fact
}

func TestAssertionFeedbackKeepsLegacyDetailedReceiptBytes(t *testing.T) {
	o := *validationFeedbackFixture().Validation
	want, got := sha256.New(), sha256.New()
	// The exact pre-extension layout, including already-archived v1 details.
	writeHashString(want, "validation-check-failure-details/v1")
	writeHashString(want, "false")
	writeHashString(want, "false")
	writeHashStrings(want, nil)
	for _, d := range o.CheckFailures {
		writeHashStrings(want, []string{d.CheckRef, d.CaseRef, d.PackageRef, d.TargetRef, fmt.Sprint(d.Line), d.DiagnosticHash})
	}
	writeRuntimeValidationFailureDetailsHash(got, o)
	if !bytes.Equal(got.Sum(nil), want.Sum(nil)) {
		t.Fatal("new diagnostic protocol changed an existing detailed receipt hash")
	}
	before, _ := json.Marshal(o)
	var reloaded RuntimeValidationObservation
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(reloaded.Clone())
	if !bytes.Equal(before, after) || strings.Contains(string(after), "comparisonHash") || strings.Contains(string(after), "reportedAssertion") || strings.Contains(string(after), "FailureDetailProtocolRef") {
		t.Fatal("legacy reload backfilled diagnostic facts")
	}
}

func TestAssertionComparisonSurvivesReloadWithoutWeakeningReceiptIdentity(t *testing.T) {
	first := validationAssertionFeedbackFixture()
	second := first
	second.ID, second.Revision, second.DispatchID = "failure-2", "evidence-2", "dispatch-2"
	second.Validation = first.Validation.ClonePointerForTest()
	second.Validation.CheckFailures[0].DiagnosticHash = strings.Repeat("d", 64)
	second.Validation.CheckFailures[0].Line++
	if RuntimeValidationFailureFingerprint(first) == "" || RuntimeValidationFailureFingerprint(first) != RuntimeValidationFailureFingerprint(second) {
		t.Fatal("owner-normalized assertion lost stable comparison")
	}
	if StableRuntimeOperationReceiptHash(RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: first.Validation}}) ==
		StableRuntimeOperationReceiptHash(RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: second.Validation}}) {
		t.Fatal("comparison identity replaced exact receipt identity")
	}
	encoded, _ := json.Marshal([]EvidenceFact{first, second})
	var restored []EvidenceFact
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if RuntimeValidationFailureOccurrences(EvidenceLedger{Rejected: append(restored, restored[0])}, second) != 2 {
		t.Fatal("reload or duplicate changed occurrence count")
	}
	legacy := validationFeedbackFixture()
	if RuntimeValidationFailureFingerprint(legacy) == RuntimeValidationFailureFingerprint(first) {
		t.Fatal("new comparison guessed equivalence with an unnormalized historical assertion")
	}
	second.Validation.CheckFailures[0].ComparisonHash = strings.Repeat("e", 64)
	if RuntimeValidationFailureFingerprint(first) == RuntimeValidationFailureFingerprint(second) {
		t.Fatal("distinct assertion merged into current failure")
	}
}

func TestAssertionFeedbackIsBoundedDetachedAndFullyHashed(t *testing.T) {
	fact := validationAssertionFeedbackFixture()
	if err := fact.Validation.ValidateFailureDetails(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeValidationObservation){
		"protocol":    func(o *RuntimeValidationObservation) { o.FailureDetailProtocolRef = "unknown" },
		"comparison":  func(o *RuntimeValidationObservation) { o.CheckFailures[0].ComparisonHash = strings.Repeat("d", 64) },
		"report code": func(o *RuntimeValidationObservation) { o.CheckFailures[0].ReportedAssertion.Code = "unknown" },
		"expected":    func(o *RuntimeValidationObservation) { o.CheckFailures[0].ReportedAssertion.ExpectedShape = "string" },
		"observed":    func(o *RuntimeValidationObservation) { o.CheckFailures[0].ReportedAssertion.ObservedShape = "null" },
	} {
		t.Run(name, func(t *testing.T) {
			state := (RuntimeStateRecord{Facts: RuntimeFacts{Evidence: EvidenceLedger{Rejected: []EvidenceFact{fact}}}}).Clone()
			changed := state.Facts.Evidence.Rejected[0]
			mutate(changed.Validation)
			if StableRuntimeEvidenceFactHash(fact) == StableRuntimeEvidenceFactHash(changed) {
				t.Fatal("diagnostic mutation ignored by evidence hash or aliased archived fact")
			}
			before := RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: fact.Validation}}
			after := RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: changed.Validation}}
			if StableRuntimeOperationReceiptHash(before) == StableRuntimeOperationReceiptHash(after) {
				t.Fatal("diagnostic mutation ignored by receipt hash")
			}
		})
	}
	for name, mutate := range map[string]func(*RuntimeValidationObservation){
		"missing protocol":   func(o *RuntimeValidationObservation) { o.FailureDetailProtocolRef = "" },
		"unknown protocol":   func(o *RuntimeValidationObservation) { o.FailureDetailProtocolRef = "future" },
		"missing comparison": func(o *RuntimeValidationObservation) { o.CheckFailures[0].ComparisonHash = "" },
		"raw comparison":     func(o *RuntimeValidationObservation) { o.CheckFailures[0].ComparisonHash = "private raw log" },
		"raw expected": func(o *RuntimeValidationObservation) {
			o.CheckFailures[0].ReportedAssertion.ExpectedShape = "private-token"
		},
		"raw actual": func(o *RuntimeValidationObservation) {
			o.CheckFailures[0].ReportedAssertion.ObservedShape = "https://private.example"
		},
		"equal shapes":   func(o *RuntimeValidationObservation) { o.CheckFailures[0].ReportedAssertion.ObservedShape = "object" },
		"wrong category": func(o *RuntimeValidationObservation) { o.CheckFailures[0].ReportedAssertion.Code = "presence_mismatch" },
		"no failure":     func(o *RuntimeValidationObservation) { o.FailedChecks = nil },
	} {
		t.Run(name, func(t *testing.T) {
			o := fact.Validation.Clone()
			mutate(&o)
			if o.ValidateFailureDetails() == nil {
				t.Fatal("invalid or unsafe diagnostic accepted")
			}
		})
	}
}

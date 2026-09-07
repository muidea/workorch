package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAbsentValidationFeedbackExtensionPreservesLegacyHashBytes(t *testing.T) {
	for _, observation := range []RuntimeValidationObservation{
		{}, {RecipeID: "legacy-recipe", PassedChecks: []string{"legacy-check"}},
		{CheckFailures: []RuntimeValidationCheckFailure{}, IncompleteChecks: []string{}},
	} {
		h := sha256.New()
		_, _ = h.Write([]byte("existing receipt/evidence hash prefix"))
		before := string(h.Sum(nil))
		writeRuntimeValidationFailureDetailsHash(h, observation)
		if string(h.Sum(nil)) != before {
			t.Fatal("absent optional fields changed legacy receipt/evidence hash bytes")
		}
		encoded, err := json.Marshal(observation)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"CheckFailures", "IncompleteChecks", "FailureDetailsTruncated", "OutputTruncated"} {
			if strings.Contains(string(encoded), field) {
				t.Fatalf("default extension wrote archive field %s", field)
			}
		}
	}
}

func validationFeedbackFixture() EvidenceFact {
	return EvidenceFact{ID: "failure-1", Kind: EvidenceKindValidation, FocusID: "focus-1",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, FailureClass: FailureClassExecutionFailure, FailureCode: "test_failed", FailureSignature: "transaction-1",
		Validation: &RuntimeValidationObservation{RecipeID: "recipe", ObservationProtocolRef: "owner/checks/v1", FailedChecks: []string{"check-A"},
			CheckFailures: []RuntimeValidationCheckFailure{{CheckRef: "check-A", CaseRef: "check-A/case", TargetRef: "checks/a.test", Line: 47, DiagnosticHash: strings.Repeat("a", 64)}}}}
}

func TestValidationFailureComparisonDoesNotWeakenTransactionFences(t *testing.T) {
	first := validationFeedbackFixture()
	second := first
	second.Validation = first.Validation.ClonePointerForTest()
	second.ID, second.DispatchID, second.Revision, second.FailureSignature = "failure-2", "new-dispatch", "new-evidence", "new-closure-signature"
	second.Validation.CheckFailures[0].Line++
	if RuntimeValidationFailureFingerprint(first) == "" || RuntimeValidationFailureFingerprint(first) != RuntimeValidationFailureFingerprint(second) {
		t.Fatal("a new repair transaction or shifted line erased the same check failure")
	}
	if StableRuntimeEvidenceFactHash(first) == StableRuntimeEvidenceFactHash(second) {
		t.Fatal("comparison weakened exact evidence identity")
	}
	if n := RuntimeValidationFailureOccurrences(EvidenceLedger{Rejected: []EvidenceFact{first, second, first}}, second); n != 2 {
		t.Fatalf("duplicate counted as new failure: %d", n)
	}
	for name, mutate := range map[string]func(*EvidenceFact){
		"different assertion": func(f *EvidenceFact) { f.Validation.CheckFailures[0].DiagnosticHash = strings.Repeat("b", 64) },
		"different CWI":       func(f *EvidenceFact) { f.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-2"} },
		"different recipe":    func(f *EvidenceFact) { f.Validation.RecipeID = "another-recipe" },
		"partial transcript":  func(f *EvidenceFact) { f.Validation.OutputTruncated = true },
	} {
		t.Run(name, func(t *testing.T) {
			changed := first
			changed.Validation = first.Validation.ClonePointerForTest()
			mutate(&changed)
			if RuntimeValidationFailureOccurrences(EvidenceLedger{Rejected: []EvidenceFact{changed}}, first) != 0 {
				t.Fatal("unrelated failure joined current failure")
			}
		})
	}
	encoded, err := json.Marshal([]EvidenceFact{first, second})
	if err != nil {
		t.Fatal(err)
	}
	var reloaded []EvidenceFact
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	if RuntimeValidationFailureOccurrences(EvidenceLedger{Rejected: reloaded}, second) != 2 {
		t.Fatal("fresh reload lost repeat identity")
	}
}

func TestRejectedFailureComparisonUsesSealedBindingWithoutGrantingCredit(t *testing.T) {
	first := validationFeedbackFixture()
	want := RuntimeValidationFailureFingerprint(first)
	first.CoverageWorkItemIDs = nil
	first.BindingRefs = []string{"binding-cwi-1"}
	binding := CoverageEvidenceBinding{ID: "binding-cwi-1", CoverageWorkItemID: "cwi-1"}
	second := first
	second.ID, second.FailureSignature = "failure-2", "new-transaction"
	ledger := EvidenceLedger{Rejected: []EvidenceFact{first, second}}
	if RuntimeValidationFailureFingerprint(first, binding) != want || RuntimeValidationFailureOccurrences(ledger, second, binding) != 2 {
		t.Fatal("rejected receipt lost its sealed CWI diagnostic attribution")
	}
	if len(first.CoverageWorkItemIDs) != 0 || len(ledger.Rejected[0].CoverageWorkItemIDs) != 0 || len(ledger.Accepted) != 0 {
		t.Fatal("comparison manufactured evidence credit")
	}
	if RuntimeValidationFailureFingerprint(first) != "" || RuntimeValidationFailureOccurrences(ledger, second, CoverageEvidenceBinding{ID: "unrelated", CoverageWorkItemID: "cwi-1"}) != 0 {
		t.Fatal("unknown binding guessed CWI diagnostic attribution")
	}
}

func TestValidationFailureComparisonResetsOnlyAfterExactCreditedPass(t *testing.T) {
	failed := validationFeedbackFixture()
	failed.Revision = "evidence-1"
	passed := failed
	passed.Revision = "evidence-2"
	passed.Validation = failed.Validation.ClonePointerForTest()
	passed.Validation.FailedChecks, passed.Validation.CheckFailures = nil, nil
	passed.Validation.PassedChecks = []string{"check-A"}
	ledger := EvidenceLedger{Rejected: []EvidenceFact{failed}, Accepted: []EvidenceFact{passed}}
	if RuntimeValidationFailureOccurrences(ledger, failed) != 0 {
		t.Fatal("resolved failure leaked into a later repair window")
	}
	for name, mutate := range map[string]func(*EvidenceFact){
		"repair":      func(f *EvidenceFact) { f.Kind = EvidenceKindStateChange },
		"uncredited":  func(f *EvidenceFact) { f.CoverageWorkItemIDs = nil },
		"other check": func(f *EvidenceFact) { f.Validation.PassedChecks = []string{"check-B"} },
		"incomplete":  func(f *EvidenceFact) { f.Validation.IncompleteChecks = []string{"check-A"} },
		"truncated":   func(f *EvidenceFact) { f.Validation.OutputTruncated = true },
	} {
		t.Run(name, func(t *testing.T) {
			changed := passed
			changed.Validation = passed.Validation.ClonePointerForTest()
			mutate(&changed)
			if RuntimeValidationFailureOccurrences(EvidenceLedger{Accepted: []EvidenceFact{changed}, Rejected: []EvidenceFact{failed}}, failed) != 1 {
				t.Fatal("non-proof erased failure history")
			}
		})
	}
}

func (o *RuntimeValidationObservation) ClonePointerForTest() *RuntimeValidationObservation {
	value := o.Clone()
	return &value
}

func TestValidationFailureDetailsAreDetachedBoundedAndHashed(t *testing.T) {
	first := validationFeedbackFixture()
	observation := first.Validation.Clone()
	if err := observation.ValidateFailureDetails(); err != nil {
		t.Fatal(err)
	}
	observation.CheckFailures[0].Line++
	if first.Validation.CheckFailures[0].Line != 47 {
		t.Fatal("normalized clone aliases owner fact")
	}
	state := RuntimeStateRecord{Facts: RuntimeFacts{Evidence: EvidenceLedger{Rejected: []EvidenceFact{first}}}}
	copy := state.Clone()
	copy.Facts.Evidence.Rejected[0].Validation.CheckFailures[0].Line++
	if first.Validation.CheckFailures[0].Line != 47 {
		t.Fatal("raw state clone aliases diagnostic details")
	}
	baseline := RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: first.Validation}}
	for name, mutate := range map[string]func(*RuntimeValidationObservation){
		"line":             func(o *RuntimeValidationObservation) { o.CheckFailures[0].Line++ },
		"check":            func(o *RuntimeValidationObservation) { o.CheckFailures[0].CheckRef = "check-B" },
		"case":             func(o *RuntimeValidationObservation) { o.CheckFailures[0].CaseRef = "another-case" },
		"package":          func(o *RuntimeValidationObservation) { o.CheckFailures[0].PackageRef = "another-package" },
		"target":           func(o *RuntimeValidationObservation) { o.CheckFailures[0].TargetRef = "other.test" },
		"digest":           func(o *RuntimeValidationObservation) { o.CheckFailures[0].DiagnosticHash = strings.Repeat("b", 64) },
		"truncated":        func(o *RuntimeValidationObservation) { o.FailureDetailsTruncated = true },
		"output truncated": func(o *RuntimeValidationObservation) { o.OutputTruncated = true },
		"incomplete check": func(o *RuntimeValidationObservation) { o.IncompleteChecks = []string{"check-A"} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := baseline.Clone()
			mutate(changed.Evidence.Validation)
			if StableRuntimeOperationReceiptHash(changed) == StableRuntimeOperationReceiptHash(baseline) {
				t.Fatal("receipt hash ignored detail drift")
			}
			fact := first
			fact.Validation = changed.Evidence.Validation
			if StableRuntimeEvidenceFactHash(first) == StableRuntimeEvidenceFactHash(fact) {
				t.Fatal("evidence hash ignored detail drift")
			}
		})
	}
	for name, mutate := range map[string]func(*RuntimeValidationObservation){
		"wrong check":    func(o *RuntimeValidationObservation) { o.CheckFailures[0].CheckRef = "passing-B" },
		"absolute":       func(o *RuntimeValidationObservation) { o.CheckFailures[0].TargetRef = "/private/a.test" },
		"escape":         func(o *RuntimeValidationObservation) { o.CheckFailures[0].TargetRef = "../a.test" },
		"raw diagnostic": func(o *RuntimeValidationObservation) { o.CheckFailures[0].DiagnosticHash = "private assertion text" },
		"duplicate":      func(o *RuntimeValidationObservation) { o.CheckFailures = append(o.CheckFailures, o.CheckFailures[0]) },
		"oversized": func(o *RuntimeValidationObservation) {
			o.CheckFailures = make([]RuntimeValidationCheckFailure, RuntimeValidationFailureDetailLimit+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := first.Validation.Clone()
			mutate(&changed)
			if changed.ValidateFailureDetails() == nil {
				t.Fatal("unsafe details accepted")
			}
		})
	}
}

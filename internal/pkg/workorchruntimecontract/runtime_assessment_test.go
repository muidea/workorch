package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeAssessmentGapStrictlyNarrowsOnlyForNonExpandingNeutralSets(t *testing.T) {
	previous := RuntimeAssessmentGap{
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"coverage-a", "coverage-b"},
		MissingEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation},
		TargetRefs:           []string{"implementation.go"},
		Reason:               "prior wording",
	}
	if !RuntimeAssessmentGapStrictlyNarrows(previous, RuntimeAssessmentGap{
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"coverage-b"},
		MissingEvidenceKinds: []string{EvidenceKindValidation},
		Reason:               "different wording",
	}) {
		t.Fatal("reduced owner coverage did not count as gap progress")
	}
	if RuntimeAssessmentGapStrictlyNarrows(previous, RuntimeAssessmentGap{
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"coverage-a", "coverage-b"},
		MissingEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation},
		Reason:               "only wording changed",
	}) {
		t.Fatal("reason-only change counted as gap progress")
	}
	if RuntimeAssessmentGapStrictlyNarrows(previous, RuntimeAssessmentGap{
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"coverage-b", "coverage-c"},
		MissingEvidenceKinds: []string{EvidenceKindValidation},
	}) {
		t.Fatal("gap with a new coverage item counted as progress")
	}
	if RuntimeAssessmentGapStrictlyNarrows(previous, RuntimeAssessmentGap{
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"coverage-a", "coverage-b"},
		MissingEvidenceKinds: []string{EvidenceKindValidation},
	}) {
		t.Fatal("LLM claimed missing-kind change counted as repair progress")
	}
}

func TestValidateAssessmentDispositionPreservesOriginalObservation(t *testing.T) {
	if _, err := ValidateAssessmentDisposition("", nil, nil); err == nil {
		t.Fatal("missing assessment disposition was accepted")
	}
	// A contradictory item list is original diagnostic content. It must not
	// rewrite the producer's claimed disposition; SM-2 later derives canonical
	// coverage from owner receipts.
	d, err := ValidateAssessmentDisposition(AssessmentDispositionSatisfied, []RuntimeAssessmentItemFact{
		{CoverageWorkItemID: "cwi-1", Satisfied: true},
		{CoverageWorkItemID: "cwi-2", Satisfied: false, Reason: "missing validation"},
	}, nil)
	if err != nil || d != AssessmentDispositionSatisfied {
		t.Fatalf("partial = %q err=%v", d, err)
	}
	// A claimed repairable gap with sparse diagnostic information remains a
	// valid observation rather than a retry/action instruction.
	d, err = ValidateAssessmentDisposition(AssessmentDispositionRepairableGap, nil, nil)
	if err != nil || d != AssessmentDispositionRepairableGap {
		t.Fatalf("sparse repairable observation = %q err=%v", d, err)
	}
	d, err = ValidateAssessmentDisposition(AssessmentDispositionRepairableGap, nil, &RuntimeAssessmentGap{
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Reason: "implementation incomplete",
	})
	if err != nil || d != AssessmentDispositionRepairableGap {
		t.Fatalf("structured gap = %q err=%v", d, err)
	}
}

func TestAssessmentCandidatePreservesOutOfSliceObservationItems(t *testing.T) {
	facts := testRuntimeFacts()
	facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
	facts.Focus.RequireAssessment = true
	decision, err := SealRuntimeDecision(facts, RuntimeDecision{
		State: ConvergenceStateNeedAssessment, Focus: facts.Focus,
		RequiredOutput: RequiredOutputContract{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	candidate := RuntimeCandidate{Assessment: &AssessmentCandidate{
		FocusID: decision.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: AssessmentDispositionRepairableGap, Summary: "partial",
		Items: []RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-9", Satisfied: false, Reason: "outside"}},
		Gap:   &RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Reason: "gap"},
	}}
	err = candidate.Validate(decision)
	if err != nil {
		t.Fatalf("original observation item should not change scope validity: %v", err)
	}
}

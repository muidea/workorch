package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateAssessmentDispositionRequiresExplicitAuthorityAndValidatesPartial(t *testing.T) {
	if _, err := ValidateAssessmentDisposition("", nil, nil); err == nil {
		t.Fatal("missing assessment disposition was accepted")
	}
	// partial items with satisfied disposition normalize to repairable_gap
	d, err := ValidateAssessmentDisposition(AssessmentDispositionSatisfied, []RuntimeAssessmentItemFact{
		{CoverageWorkItemID: "cwi-1", Satisfied: true},
		{CoverageWorkItemID: "cwi-2", Satisfied: false, Reason: "missing validation"},
	}, nil)
	if err != nil || d != AssessmentDispositionRepairableGap {
		t.Fatalf("partial = %q err=%v", d, err)
	}
	// repairable gap requires structure
	_, err = ValidateAssessmentDisposition(AssessmentDispositionRepairableGap, nil, nil)
	if err == nil {
		t.Fatal("expected repairable_gap without structure to fail")
	}
	d, err = ValidateAssessmentDisposition(AssessmentDispositionRepairableGap, nil, &RuntimeAssessmentGap{
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Reason: "implementation incomplete",
	})
	if err != nil || d != AssessmentDispositionRepairableGap {
		t.Fatalf("structured gap = %q err=%v", d, err)
	}
}

func TestAssessmentCandidateRejectsOutOfSliceItems(t *testing.T) {
	facts := testRuntimeFacts()
	facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
	facts.Focus.RequireAssessment = true
	decision, err := SealRuntimeDecision(facts, RuntimeDecision{
		State: ConvergenceStateNeedAssessment, Action: RuntimeActionProduceAssessment, Focus: facts.Focus,
		RequiredOutput: RequiredOutputContract{Kind: RequiredOutputKindAssessment, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
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
	if err == nil || !strings.Contains(err.Error(), "outside current slice coverage") {
		t.Fatalf("expected out-of-slice rejection, got %v", err)
	}
}

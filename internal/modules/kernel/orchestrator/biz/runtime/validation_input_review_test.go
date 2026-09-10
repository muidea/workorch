package runtime

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInputReviewInformationBudgetAppliesOnlyToUnresolvedExactSubjects(t *testing.T) {
	document := runtimecontract.RuntimeSemanticPlanDocument{Proposals: []runtimecontract.RuntimeSemanticProofProposal{
		{ValidationInputReview: &runtimecontract.RuntimeValidationInputReview{SubjectHash: "unresolved-input", Checks: []runtimecontract.RuntimeValidationInputCheckReview{{Conclusion: runtimecontract.RuntimeValidationInputInconclusive}}}},
		{ValidationInputReview: &runtimecontract.RuntimeValidationInputReview{SubjectHash: "consistent-input", Checks: []runtimecontract.RuntimeValidationInputCheckReview{{Conclusion: runtimecontract.RuntimeValidationInputConsistent}}}},
	}}
	for _, tc := range []struct {
		name, subject, want string
		count               int
	}{
		{name: "first question"},
		{name: "last available retirement", count: runtimecontract.RuntimeSemanticPlanningHistoryLimit - 1},
		{name: "history exhausted", count: runtimecontract.RuntimeSemanticPlanningHistoryLimit, want: "validation_input_review_history_exhausted"},
		{name: "exact unresolved subject already answered", subject: "unresolved-input", want: "validation_input_review_information_exhausted"},
		{name: "different content has a new subject", subject: "older-input"},
		{name: "another now-consistent input does not block a new question", subject: "consistent-input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var info []runtimecontract.RuntimeValidationInputReviewInformation
			if tc.subject != "" {
				info = []runtimecontract.RuntimeValidationInputReviewInformation{{SubjectHash: tc.subject}}
			}
			if got := runtimeInputReviewInformationFailure(tc.count, info, document); got != tc.want {
				t.Fatalf("failure=%q want %q", got, tc.want)
			}
		})
	}
	for _, order := range [][]string{{runtimecontract.RuntimeValidationInputConflict, runtimecontract.RuntimeValidationInputInconclusive}, {runtimecontract.RuntimeValidationInputInconclusive, runtimecontract.RuntimeValidationInputConflict}} {
		document.Proposals[0].ValidationInputReview.Checks[0].Conclusion = order[0]
		document.Proposals[1].ValidationInputReview.Checks[0].Conclusion = order[1]
		if runtimeInputReviewConclusion(document) != runtimecontract.RuntimeValidationInputConflict {
			t.Fatal("uncertainty overrode a known conflict")
		}
	}
}

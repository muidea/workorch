package tui

import (
	"reflect"
	"strings"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInspectAcceptancePreservesCompleteInputReviewAndDoesNotChooseActions(t *testing.T) {
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, OriginalCode: "validation_input_review_coverage_incomplete", ValidationInputReviews: []runtimecontract.RuntimeValidationInputReviewFeedback{{
		ReviewRef: "review-1", ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2, CoverageWorkItemID: "cwi-1", RequirementHash: "original-requirement", ReportHash: "original-report", Redacted: true,
		Report: runtimecontract.RuntimeValidationInputReview{SubjectHash: "exact-input", Checks: []runtimecontract.RuntimeValidationInputCheckReview{{
			RequiredCheckRef: "original-check", Conclusion: runtimecontract.RuntimeValidationInputConflict, Reason: strings.Repeat("complete diagnostic ", 20) + "final detail", InputTargetRefs: []string{"checks/input"},
			Obligations: []runtimecontract.RuntimeValidationInputObligationReview{{CriterionText: "document tools", Finding: runtimecontract.RuntimeValidationInputMissing, Reason: "no documentation assertion", InputTargetRefs: []string{"checks/input"}}},
		}}},
	}}}
	before := diagnostic.CloneOriginal()
	text := renderInspectAcceptance(projectioncontract.AcceptanceProjection{Audit: &projectioncontract.RuntimeAudit{Failure: &projectioncontract.RuntimeAuditFailure{OriginalDiagnostic: diagnostic}}})
	for _, want := range []string{"diagnostic only, not validation evidence", "review-1", "cwi-1", "original-check", "original-requirement", "exact-input", "original-report", "final detail", "document tools", "missing", "no documentation assertion", "checks/input", "redacted"} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspect view lost %q", want)
		}
	}
	if !reflect.DeepEqual(before, diagnostic) || strings.Contains(text, "canonical repair directive") {
		t.Fatal("diagnostic rendering changed facts or invented repair authority")
	}
}

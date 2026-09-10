package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestInputReviewFeedbackPreservesExactReportsAndRedactsUnsafeText(t *testing.T) {
	_, document, binding := semanticInputReviewV2Fixture(t)
	report := document.Proposals[1].ValidationInputReview.Clone()
	feedback, err := ProjectRuntimeValidationInputReviewFeedback("runtime-input-review-source", binding, report)
	if err != nil || feedback.Redacted || !reflect.DeepEqual(feedback.Report, report) {
		t.Fatalf("safe complete report lost: %v", err)
	}
	diagnostic := &RuntimeOriginalDiagnostic{SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleOrchestrator, OriginalClass: "contract", OriginalCode: "validation_input_review_conflict", OriginalMessage: "Input review blocked validation.", Signature: feedback.ReviewRef, ValidationInputReviews: []RuntimeValidationInputReviewFeedback{feedback}}
	if err := diagnostic.Validate(); err != nil {
		t.Fatal(err)
	}
	before := StableRuntimeFeedbackHash(RuntimeFeedback{OriginalDiagnostic: diagnostic})
	copy := diagnostic.CloneOriginal()
	copy.ValidationInputReviews[0].Report.Checks[0].Obligations[0].Reason = "changed"
	if diagnostic.ValidationInputReviews[0].Report.Checks[0].Obligations[0].Reason == "changed" || StableRuntimeFeedbackHash(RuntimeFeedback{OriginalDiagnostic: copy}) == before || copy.Validate() == nil {
		t.Fatal("nested feedback clone/hash did not preserve exact report")
	}
	legacy := diagnostic.CloneOriginal()
	legacy.ValidationInputReviews = nil
	data, _ := json.Marshal(legacy)
	if strings.Contains(string(data), "validationInputReviews") {
		t.Fatal("empty extension changed legacy diagnostic JSON")
	}
	report.Checks[0].Reason = "authorization: Bearer secret-value"
	report.Checks[0].Obligations[0].Reason = "https://private.example/secret-value"
	redacted, err := ProjectRuntimeValidationInputReviewFeedback(feedback.ReviewRef, binding, report)
	if err != nil || !redacted.Redacted || redacted.ReportHash != StableRuntimeValidationInputReviewHash(report) || redacted.ReportHash == feedback.ReportHash {
		t.Fatalf("redaction lost original source commitment: %v", err)
	}
	data, _ = json.Marshal(redacted)
	if strings.Contains(string(data), "secret-value") || strings.Contains(string(data), "private.example") || strings.Contains(string(data), "Files") || strings.Contains(string(data), "package checks") {
		t.Fatal("input bytes or sensitive report text leaked into feedback")
	}
	if !strings.Contains(report.Checks[0].Reason, "secret-value") {
		t.Fatal("feedback redaction rewrote the original archived report")
	}
	for _, mutate := range []func(*RuntimeValidationInputReviewFeedback){
		func(f *RuntimeValidationInputReviewFeedback) { f.Report.Checks[0].Obligations = nil },
		func(f *RuntimeValidationInputReviewFeedback) { f.Report.Checks[0].Obligations[0].Finding = "approve" },
		func(f *RuntimeValidationInputReviewFeedback) {
			f.Report.Checks[0].Obligations[0].Finding = RuntimeValidationInputMissing
		},
		func(f *RuntimeValidationInputReviewFeedback) {
			f.Report.Checks[0].Obligations[0].InputTargetRefs = []string{"foreign-input"}
		},
	} {
		bad := redacted.Clone()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("redaction bypassed the closed report shape and conclusion")
		}
	}
	report.Checks[0].Reason = "unsafe terminal sequence: \x1b[31m"
	controlled, err := ProjectRuntimeValidationInputReviewFeedback(feedback.ReviewRef, binding, report)
	if err != nil || !controlled.Redacted || strings.ContainsRune(controlled.Report.Checks[0].Reason, '\x1b') {
		t.Fatalf("unsafe control characters leaked into a terminal projection: %v", err)
	}
	copy = diagnostic.CloneOriginal()
	copy.OwnerModuleID = RuntimeModuleShell
	if copy.Validate() == nil {
		t.Fatal("capability owner forged an admitted report projection")
	}
}

func TestInputReviewProtocolIsExplicitAndRecipeCommitmentsCannotDowngrade(t *testing.T) {
	policy := RuntimeEvidencePolicy{ID: "review", AppliesTo: StepKindShellCommand, RecipeDeclared: true, RecipeAdapterRef: "owner/review/v1"}
	legacy, _ := json.Marshal(policy)
	if strings.Contains(string(legacy), "RecipeInputReviewProtocolRef") {
		t.Fatal("new policy field changed legacy activation JSON")
	}
	old := RuntimeValidationRecipeDeclarationsFromEvidencePolicies([]RuntimeEvidencePolicy{policy})[0]
	policy.RecipeInputReviewProtocolRef = RuntimeValidationInputReviewProtocolV2
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	current := RuntimeValidationRecipeDeclarationsFromEvidencePolicies([]RuntimeEvidencePolicy{policy})[0]
	old.Command = "check"
	if current.InputReviewProtocolRef != policy.RecipeInputReviewProtocolRef || runtimeValidationRecipeConfirmsDeclaration(current, old) {
		t.Fatal("declaration silently dropped or downgraded the review protocol")
	}
	for _, mutate := range []func(*RuntimeEvidencePolicy){
		func(p *RuntimeEvidencePolicy) { p.RecipeDeclared = false },
		func(p *RuntimeEvidencePolicy) { p.RecipeAdapterRef = ""; p.CommandName = "check" },
		func(p *RuntimeEvidencePolicy) { p.RecipeInputReviewProtocolRef = "unsupported" },
	} {
		bad := policy
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("unsupported or undeclared review protocol accepted")
		}
	}
}

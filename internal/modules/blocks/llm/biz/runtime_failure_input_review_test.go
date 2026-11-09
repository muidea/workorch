package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFailureInputReviewProviderRetainsExactInputsAndReceiptBody(t *testing.T) {
	request := runtimeInputReviewProviderFixture(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	m, c := &request.SemanticManifest, &request.SemanticContext
	input := m.OwnerCatalogs[0].ValidationSurfaces[0].ProofRequirementBindings[0].InputReview
	body := "expected 15, got 13\n        exact native continuation"
	digest := sha256.Sum256([]byte(body))
	f := rc.RuntimeSemanticExecutionFeedback{WorkspaceID: c.WorkspaceID, RunID: c.RunID, DefinitionRevision: c.Definition.Revision, StartupInputHash: c.Definition.StartupInputHash, ManifestHash: m.Hash, CoverageWorkItemID: m.CoverageItems[0].ID, FailedFocusID: "original-focus", FailureSignature: "failure", OwnerModuleID: rc.RuntimeModuleShell, OperationKind: rc.RuntimeBlockKindManagedValidationSession, EvidenceID: "failure-event", EvidenceRevision: "evidence:1", DispatchID: "dispatch", OperationID: "operation", ReceiptRef: "receipt", ReceiptHash: "receipt-hash",
		Diagnostic: &rc.RuntimeSemanticExecutionDiagnostic{FeedbackHash: "owner-feedback", OwnerModuleID: rc.RuntimeModuleShell, OriginalClass: "execution_failure", OriginalCode: "validation_failed", Signature: "failure", SummaryOnly: true},
		Validation: rc.ProjectRuntimeValidationExecutionFeedback(&rc.RuntimeValidationObservation{RecipeID: "review-recipe", FailedChecks: []string{"check"}, FailureDetailProtocolRef: rc.RuntimeValidationFailureDetailProtocol, CheckFailures: []rc.RuntimeValidationCheckFailure{{CheckRef: "check", TargetRef: "checks/input_test.go", Line: 15, DiagnosticHash: hex.EncodeToString(digest[:]), ComparisonHash: hex.EncodeToString(digest[:]), AssertionText: body}}})}
	// Preserve a real-size failure set through the provider projection, complete
	// envelope gate, frozen reload and rendered integrity; no hidden count32 cap.
	for n := 1; n < 39; n++ {
		detail := f.Validation.CheckFailures[0]
		detail.CaseRef = "check/" + strings.Repeat("native_case_", 12) + fmt.Sprintf("%03d", n)
		detail.Line += n
		f.Validation.CheckFailures = append(f.Validation.CheckFailures, detail)
	}
	binding := m.OwnerCatalogs[0].ValidationSurfaces[0].ProofRequirementBindings[0]
	report := rc.RuntimeValidationInputReview{SubjectHash: input.Hash}
	for _, check := range binding.RequiredCheckRefs {
		report.Checks = append(report.Checks, rc.RuntimeValidationInputCheckReview{RequiredCheckRef: check, Conclusion: rc.RuntimeValidationInputConsistent, Reason: "previous formal input review", InputTargetRefs: []string{input.Files[0].TargetRef}, Obligations: []rc.RuntimeValidationInputObligationReview{{CriterionText: m.CoverageItems[0].TargetText, Finding: rc.RuntimeValidationInputCovered, Reason: "original criterion preserved", InputTargetRefs: []string{input.Files[0].TargetRef}}}})
	}
	previous, err := rc.ProjectRuntimeValidationInputReviewFeedback("runtime-input-review-original", binding, report)
	if err != nil {
		t.Fatal(err)
	}
	f.PreviousInputReview = &previous
	f.Hash = rc.StableRuntimeSemanticExecutionFeedbackHash(f)
	m.FailureInputReview = &rc.RuntimeValidationFailureInputReview{BindingRef: rc.RuntimeCWIBindingRef{BindingID: "binding", BindingHash: "binding-hash", FocusID: "original-focus", SemanticManifestHash: m.Hash}, SubjectHash: input.Hash, FailureFingerprint: "fingerprint", EvidenceHash: "evidence-hash", Feedback: f}
	m.Hash = rc.StableRuntimeSemanticPlanManifestHash(*m)
	f.ManifestHash = m.Hash
	f.Hash = rc.StableRuntimeSemanticExecutionFeedbackHash(f)
	c.ExecutionFeedback = []rc.RuntimeSemanticExecutionFeedback{f}
	refreshSemanticProgressForTest(t, &request)
	rendered, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	var projected runtimeSemanticTailProjection
	if err := json.Unmarshal([]byte(rendered.PromptContext.Sections[4].Content), &projected); err != nil || len(projected.ExecutionFeedback) != 1 || len(projected.ExecutionFeedback[0].Validation.CheckFailures) != 39 {
		t.Fatal("39 failures did not reach provider unchanged", err)
	}
	for i := 0; i < 3; i++ {
		if before.PromptContext.Sections[i].Content != rendered.PromptContext.Sections[i].Content {
			t.Fatalf("failure review changed stable C%d", i)
		}
	}
	if !strings.Contains(rendered.PromptContext.Sections[4].Content, "previous formal input review") || !strings.Contains(rendered.PromptContext.Sections[3].Content, "failureInputReviewSubjectHash") || !strings.Contains(rendered.PromptContext.Sections[4].Content, `expected 15, got 13\n        exact native continuation`) {
		t.Fatal("source or exact assertion missing")
	}
	for i := 0; i < 4; i++ {
		if strings.Contains(rendered.PromptContext.Sections[i].Content, "expected 15, got 13") {
			t.Fatalf("failure body leaked into C%d", i)
		}
	}
	// Frozen detached request replay must retain every native byte and identity.
	raw, _ := json.Marshal(request)
	var frozen llmmodels.RuntimeCandidateRequest
	if err := json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	replay, err := runtimeProviderRequest(frozen)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rendered.PromptContext.Sections {
		if replay.PromptContext.Sections[i].Content != rendered.PromptContext.Sections[i].Content {
			t.Fatal("frozen replay re-render drift")
		}
	}
	for name, mutate := range map[string]func(*runtimeSemanticTailProjection){
		"assertion": func(tail *runtimeSemanticTailProjection) {
			tail.ExecutionFeedback[0].Validation.CheckFailures[0].AssertionText = "another assertion"
		},
		"previous report": func(tail *runtimeSemanticTailProjection) {
			tail.ExecutionFeedback[0].PreviousInputReview.Report.Checks[0].Reason = "another formal report"
		},
	} {
		t.Run(name, func(t *testing.T) {
			var tail runtimeSemanticTailProjection
			if err := json.Unmarshal([]byte(replay.PromptContext.Sections[4].Content), &tail); err != nil {
				t.Fatal(err)
			}
			mutate(&tail)
			mutated, _ := json.Marshal(tail)
			rendered.PromptContext.Sections[4].Content = string(mutated)
			if validateRuntimeSemanticContextIntegrity(*c, *m, 0, nil, nil, rendered.ContextBudget, rendered.PromptContext) == nil {
				t.Fatal("changed source body passed rendered integrity")
			}
		})
	}
}

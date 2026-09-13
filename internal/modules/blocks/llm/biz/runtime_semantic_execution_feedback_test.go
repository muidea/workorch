package llm

import (
	"encoding/json"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func refreshSemanticProgressForTest(t testing.TB, request *llmmodels.RuntimeCandidateRequest) {
	t.Helper()
	c := request.SemanticContext
	p, err := runtimecontract.BuildRuntimeSemanticCoverageProgress(c, request.SemanticManifest, runtimecontract.RuntimeFacts{
		WorkspaceID: c.WorkspaceID, RunID: c.RunID, DefinitionRevision: c.Definition.Revision,
		DefinitionSeed: c.DefinitionSeed, EvidenceRevision: "evidence-1", Coverage: runtimecontract.InitialRuntimeCoverageFacts(c.DefinitionSeed.CoverageItems),
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	request.SemanticContext.CoverageProgress = p
	request.SemanticContext.ExecutionHistory, err = runtimecontract.BuildRuntimeSemanticExecutionHistory(request.SemanticContext, request.SemanticManifest, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	budgetView, err := runtimecontract.BuildRuntimeExecutionBudgetView(c.WorkspaceID, c.RunID, p.SourceStateRevision, runtimecontract.RuntimeExecutionBudgetLedger{Policy: runtimecontract.DefaultRuntimeExecutionBudgetPolicy()}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	request.SemanticContext.ExecutionBudget, err = runtimecontract.BuildRuntimeSemanticExecutionBudget(request.SemanticContext, request.SemanticManifest, budgetView, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func runtimeSemanticRepairFeedbackForTest(c runtimecontract.RuntimeSemanticPlanRequestContext, m runtimecontract.RuntimeSemanticPlanManifest) []runtimecontract.RuntimeSemanticExecutionFeedback {
	var ret []runtimecontract.RuntimeSemanticExecutionFeedback
	for _, obligation := range m.ProofObligations {
		r := obligation.ValidationFailureRepair
		if r == nil {
			continue
		}
		f := runtimecontract.RuntimeSemanticExecutionFeedback{
			WorkspaceID: c.WorkspaceID, RunID: c.RunID, DefinitionRevision: c.Definition.Revision, StartupInputHash: c.Definition.StartupInputHash,
			ManifestHash: m.Hash, CoverageWorkItemID: obligation.CoverageWorkItemID, FailedFocusID: r.FailedFocusID, FailureSignature: r.FailureSignature,
			OwnerModuleID: r.TriggerOwnerModuleID, OperationKind: r.TriggerOperationKind,
			EvidenceID: "evidence-1", EvidenceRevision: "evidence:1", DispatchID: "dispatch-1", OperationID: "validation-1", ReceiptRef: "receipt-1", ReceiptHash: "receipt-hash-1",
			Diagnostic: &runtimecontract.RuntimeSemanticExecutionDiagnostic{FeedbackHash: "feedback-hash", OwnerModuleID: r.TriggerOwnerModuleID,
				OriginalClass: "contract", OriginalCode: "validation_failed", Signature: r.FailureSignature, SummaryOnly: true},
			Validation: runtimecontract.ProjectRuntimeValidationExecutionFeedback(&runtimecontract.RuntimeValidationObservation{
				RecipeID: "recipe/validation", ExecutedChecks: []string{"check/behavior"}, FailedChecks: []string{"check/behavior"},
				OutputPreview: "RAW_SECRET_MUST_NOT_TRAVEL",
			}),
		}
		f.Hash = runtimecontract.StableRuntimeSemanticExecutionFeedbackHash(f)
		ret = append(ret, f)
	}
	return ret
}

func TestRuntimeSemanticFeedbackIntegrityRejectsPostRenderMutation(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	provider, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	// An ordinary proof has no repair provenance and must never receive an
	// unrelated failure even if that packet has a self-consistent hash.
	var tail runtimeSemanticTailProjection
	if err := json.Unmarshal([]byte(provider.PromptContext.Sections[4].Content), &tail); err != nil {
		t.Fatal(err)
	}
	tail.ExecutionFeedback = []runtimecontract.RuntimeSemanticExecutionFeedback{{ReceiptRef: "unrelated"}}
	data, _ := json.Marshal(tail)
	provider.PromptContext.Sections[4].Content = string(data)
	err = validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, request.CandidateAttempt,
		request.SemanticCorrectionObservation, request.SemanticCorrectionDirective, provider.ContextBudget, provider.PromptContext)
	if err == nil || !strings.Contains(err.Error(), "P1.semantic_execution_feedback") {
		t.Fatalf("post-render injected feedback was not rejected: %v", err)
	}
}

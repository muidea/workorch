package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeSemanticProgressChangesOnlyVolatileContext(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	p := request.SemanticContext.CoverageProgress.Clone()
	p.SourceStateRevision++
	p.VisibleItems[0].RecordedEvidenceCount = 3
	p.VisibleItems[0].ReopenCount = 1
	p.Hash = runtimecontract.StableRuntimeSemanticCoverageProgressHash(p)
	request.SemanticContext.CoverageProgress = p
	request.SemanticContext.ExecutionHistory.SourceStateRevision = p.SourceStateRevision
	request.SemanticContext.ExecutionHistory.Hash = runtimecontract.StableRuntimeSemanticExecutionHistoryHash(request.SemanticContext.ExecutionHistory)
	request.SemanticContext.ExecutionBudget.SourceStateRevision = p.SourceStateRevision
	request.SemanticContext.ExecutionBudget.Hash = runtimecontract.StableRuntimeSemanticExecutionBudgetHash(request.SemanticContext.ExecutionBudget)
	after, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if !reflect.DeepEqual(before.PromptContext.Sections[i], after.PromptContext.Sections[i]) {
			t.Fatalf("progress changed C%d", i)
		}
	}
	if before.ContextBudget.CachePlan.StablePrefixHash != after.ContextBudget.CachePlan.StablePrefixHash ||
		before.PromptContext.Sections[4].Content == after.PromptContext.Sections[4].Content {
		t.Fatal("progress did not remain confined to volatile context")
	}
	var tail runtimeSemanticTailProjection
	if err := json.Unmarshal([]byte(after.PromptContext.Sections[4].Content), &tail); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tail.CoverageProgress, p) || p.Satisfied != 0 || p.VisibleItems[0].ObservedState != runtimecontract.RuntimeCoverageItemStateOpen {
		t.Fatal("recorded work changed acceptance or was lost during rendering")
	}
}

func TestRuntimeSemanticProgressIntegrityRejectsPostRenderLossOrMutation(t *testing.T) {
	for _, field := range []string{"missing", "count", "scope", "revision", "items"} {
		t.Run(field, func(t *testing.T) {
			request := runtimeSemanticProviderTestRequest(t)
			provider, err := runtimeProviderRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			var tail runtimeSemanticTailProjection
			if err := json.Unmarshal([]byte(provider.PromptContext.Sections[4].Content), &tail); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "missing":
				tail.CoverageProgress = runtimecontract.RuntimeSemanticCoverageProgress{}
			case "count":
				tail.CoverageProgress.VisibleItems[0].RecordedEvidenceCount++
			case "scope":
				tail.CoverageProgress.RunID = "foreign"
			case "revision":
				tail.CoverageProgress.SourceStateRevision++
			case "items":
				tail.CoverageProgress.VisibleItems = nil
			}
			tail.CoverageProgress.Hash = runtimecontract.StableRuntimeSemanticCoverageProgressHash(tail.CoverageProgress)
			data, _ := json.Marshal(tail)
			provider.PromptContext.Sections[4].Content = string(data)
			err = validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, request.CandidateAttempt,
				request.SemanticCorrectionObservation, request.SemanticCorrectionDirective, provider.ContextBudget, provider.PromptContext)
			if err == nil || !strings.Contains(err.Error(), "P2.semantic_coverage_progress") {
				t.Fatalf("post-render progress mismatch accepted: %v", err)
			}
		})
	}
}

func TestRuntimeSemanticProgressIsRequiredBeforeRendering(t *testing.T) {
	for _, field := range []string{"missing", "foreign"} {
		t.Run(field, func(t *testing.T) {
			request := runtimeSemanticProviderTestRequest(t)
			if field == "missing" {
				request.SemanticContext.CoverageProgress = runtimecontract.RuntimeSemanticCoverageProgress{}
			} else {
				request.SemanticContext.CoverageProgress.RunID = "foreign"
				request.SemanticContext.CoverageProgress.Hash = runtimecontract.StableRuntimeSemanticCoverageProgressHash(request.SemanticContext.CoverageProgress)
			}
			if _, err := runtimeProviderRequest(request); err == nil {
				t.Fatal("missing or wrong-scope progress accepted before provider rendering")
			}
		})
	}
}

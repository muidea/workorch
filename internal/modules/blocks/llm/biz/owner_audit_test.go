package llm

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
	"testing"
	"time"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestProviderContextBudgetAuditRetainsBoundedIntegrityMetadata(t *testing.T) {
	longWarning := strings.Repeat("w", llmAuditStringByteLimit+64)
	budget := &llmmodels.ProviderContextBudget{
		IncludedSections:  []string{"C0.runtime", "C3.authority"},
		RedactedPaths:     []string{"activation.memories[0].content"},
		ProtectedFields:   []string{"P0.goalDefinition", "P4.targetSnapshots"},
		IntegrityWarnings: []string{longWarning},
		SourceRefs:        []string{"runtime://authority/snapshot-1"},
		CachePlan: &llmmodels.ProviderCachePlan{SectionHashes: []llmmodels.ProviderCacheSectionHash{{
			SectionID: "C3.runtime-authority", Hash: "sha256:authority", SizeBytes: 55,
			SourceRefs: []string{"runtime://focus/focus-1"},
		}}},
	}
	got := providerContextBudgetAudit(budget)
	if len(got.IncludedSections) != 2 || len(got.ProtectedFields) != 2 || len(got.RedactedPaths) != 1 || len(got.SourceRefs) != 1 {
		t.Fatalf("audit context metadata = %+v", got)
	}
	if len(got.IntegrityWarnings) != 1 || !strings.HasSuffix(got.IntegrityWarnings[0], "...[truncated]") {
		t.Fatalf("bounded warning = %q", got.IntegrityWarnings)
	}
	if len(got.SectionHashes) != 1 || got.SectionHashes[0].ID != "C3.runtime-authority" || got.SectionHashes[0].SizeBytes != 55 || len(got.SectionHashes[0].SourceRefs) != 1 {
		t.Fatalf("audit section metadata = %+v", got.SectionHashes)
	}
}

func TestProviderCacheAuditMarksUnavailableAndBoundedMetadata(t *testing.T) {
	if state := providerContextBudgetAudit(&llmmodels.ProviderContextBudget{}).CacheObservation.MetadataState; state != "unavailable" {
		t.Fatalf("missing plan state %s", state)
	}
	budget := &llmmodels.ProviderContextBudget{CachePlan: &llmmodels.ProviderCachePlan{}}
	for i := 0; i < llmAuditSliceLimit+1; i++ {
		budget.CachePlan.SectionHashes = append(budget.CachePlan.SectionHashes, llmmodels.ProviderCacheSectionHash{SectionID: fmt.Sprintf("C%d", i), Hash: "hash", SizeBytes: 1})
	}
	got := providerContextBudgetAudit(budget).CacheObservation
	if got.MetadataState != "bounded" || got.SourceSectionCount != 25 || got.RecordedSectionCount != 24 {
		t.Fatalf("bounded metadata claims completeness: %+v", got)
	}
	budget.CachePlan.SectionHashes = budget.CachePlan.SectionHashes[:1]
	budget.CachePlan.SectionHashes[0].Hash = strings.Repeat("a", llmAuditStringByteLimit+1)
	if state := providerContextBudgetAudit(budget).CacheObservation.MetadataState; state != "bounded" {
		t.Fatalf("clipped identity state %s", state)
	}
}

func TestProviderCacheAuditDoesNotClaimMissingHashComplete(t *testing.T) {
	budget := &llmmodels.ProviderContextBudget{CachePlan: &llmmodels.ProviderCachePlan{SectionHashes: []llmmodels.ProviderCacheSectionHash{{SectionID: "C0", SizeBytes: 4}}}}
	if state := providerContextBudgetAudit(budget).CacheObservation.MetadataState; state != "incomplete" {
		t.Fatalf("missing hash state %s", state)
	}
}

func TestProviderAuditStageComesFromSealedRuntimeRequest(t *testing.T) {
	for _, semantic := range []bool{false, true} {
		request := llmmodels.RuntimeCandidateRequest{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}}
		want := llmaudit.PromptStage(runtimecontract.ConvergenceStateNeedStateChange)
		if semantic {
			request.DocumentKind = runtimecontract.RuntimeProviderDocumentSemanticPlan
			request.SemanticContext.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
			want = llmaudit.PromptStage(runtimecontract.RuntimeSemanticPlanStepProofBatch)
		}
		audit := sealedAuditFromRequest(request, "request", &llmmodels.ProviderContextBudget{ProviderID: "provider", PromptRenderer: "renderer", RenderContractRef: "contract"}, time.Time{}, time.Time{}, false, "failed", "preflight", 0)
		observation := providerRequestContextBudgetAudit(audit).CacheObservation
		if observation.Stage != want || observation.RenderContractRef != "contract" {
			t.Fatalf("sealed owner audit stage/render contract lost: %+v", observation)
		}
	}
}

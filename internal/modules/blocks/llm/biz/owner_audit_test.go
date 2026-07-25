package llm

import (
	"strings"
	"testing"

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

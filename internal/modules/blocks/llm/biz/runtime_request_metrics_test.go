package llm

import (
	"encoding/json"
	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
	"testing"
)

func TestRuntimeRequestMetricsMeasureLosslessReferencesAndKeepAuditDetached(t *testing.T) {
	text := strings.Repeat("完整原文\n", 500)
	p := runtimeCandidateDefinitionProjection{GoalDefinition: text, SourceObligations: &rc.RuntimeSourceObligationInventory{Sources: []rc.RuntimeDefinitionSource{{Text: text}}, Requirements: []rc.RuntimeSourceRequirement{{Text: text}}}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	metrics := runtimeProviderSectionMetrics(models.PromptSection{ID: "C1.goal-dod", Content: string(raw)})
	if metrics.BodyBytes != len(raw) || metrics.ExpandedBytes <= metrics.BodyBytes || metrics.LocalReferences != 2 || len(metrics.Components) == 0 {
		t.Fatalf("invalid byte accounting: %+v", metrics)
	}
	budget := &models.ProviderContextBudget{CachePlan: &models.ProviderCachePlan{SectionHashes: []models.ProviderCacheSectionHash{metrics}}}
	cloned := cloneProviderContextBudget(budget)
	cloned.CachePlan.SectionHashes[0].Components[0].Path = "changed"
	if budget.CachePlan.SectionHashes[0].Components[0].Path == "changed" {
		t.Fatal("audit aliases mutable caller")
	}
	for _, id := range []string{"unrelated", "C1.goal-dod"} {
		observed := runtimeProviderSectionMetrics(models.PromptSection{ID: id, Content: `{"business secret used as key":123}`})
		if len(observed.Components) != 0 {
			t.Fatal("arbitrary business key entered audit")
		}
	}
}

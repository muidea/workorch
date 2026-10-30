package llmaudit

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestContextBudgetPayloadCarriesBoundedSectionIntegrityMetadata(t *testing.T) {
	budget := ContextBudget{
		ProviderID: "ai-proxy", ModelID: "model-a", ProviderSubmitted: false,
		IncludedSections:  []string{"C0.runtime", "C1.goal-dod", "C3.authority"},
		OmittedSections:   []string{"C5.large-refs"},
		RedactedPaths:     []string{"activation.memories[0].content"},
		ProtectedFields:   []string{"P0.goalDefinition", "P4.targetSnapshots"},
		IntegrityWarnings: []string{"P4 target snapshot missing"},
		SourceRefs:        []string{"runtime://definition/rev-1", "runtime://authority/snapshot-1"},
		SectionHashes: []ContextSection{{
			ID: "C3.runtime-authority", Hash: "sha256:authority", SizeBytes: 42, SourceRefs: []string{"runtime://focus/focus-1"},
		}},
	}
	payload := budget.Payload()
	if submitted, ok := payload.Bool("providerSubmitted"); !ok || submitted {
		t.Fatalf("providerSubmitted = %v, %v", submitted, ok)
	}
	processing, _ := payload.Object("processing")
	if values, ok := processing.StringSlice("protectedFields"); !ok || len(values) != 2 || values[1] != "P4.targetSnapshots" {
		t.Fatalf("protectedFields = %v, %v", values, ok)
	}
	if values, ok := processing.StringSlice("sourceRefs"); !ok || len(values) != 2 || values[0] != "runtime://definition/rev-1" {
		t.Fatalf("sourceRefs = %v, %v", values, ok)
	}
	for _, forbidden := range []string{"prompt", "snapshotContent", "credential", "endpoint"} {
		if _, found := payload.Field(forbidden); found {
			t.Fatalf("audit payload leaked forbidden %q: %+v", forbidden, payload)
		}
	}
	field, ok := payload.Field("sectionHashes")
	if !ok || len(field.Array) != 1 {
		t.Fatalf("sectionHashes = %+v, %v", field, ok)
	}
	section := field.Array[0]
	if section.Object == nil {
		t.Fatalf("section hash is not an object: %+v", section)
	}
	sectionPayload := workorcheventpayload.Payload{Fields: section.Object}
	if id, ok := sectionPayload.String("id"); !ok || id != "C3.runtime-authority" {
		t.Fatalf("section id = %q, %v", id, ok)
	}
}

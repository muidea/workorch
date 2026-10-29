package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidationGenerationContextPreservesExactBytesAndRejectsIncompleteClaims(t *testing.T) {
	raw := "package delivery\r\n// 原文\n"
	base := RuntimeValidationGenerationContext{Version: RuntimeValidationGenerationContextVersion, ContractRef: "owner/project-context/v1", ObservedRevision: "r1", Outcome: "complete", Files: []RuntimeValidationInputReviewFile{{TargetRef: "delivery/source.txt", Content: raw, ContentHash: RuntimeValidationInputContentHash(raw)}}}
	if err := base.Validate("r1"); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(base)
	var restored RuntimeValidationGenerationContext
	if json.Unmarshal(encoded, &restored) != nil || !reflect.DeepEqual(base, restored) {
		t.Fatal("reload changed exact context")
	}
	cloned := base.Clone()
	cloned.Files[0].Content = "changed"
	if base.Files[0].Content != raw {
		t.Fatal("context clone aliases source")
	}
	for _, scenario := range []string{"hash", "duplicate", "revision", "partial", "unknown", "capacity", "path"} {
		t.Run(scenario, func(t *testing.T) {
			c := base.Clone()
			switch scenario {
			case "hash":
				c.Files[0].Content += "changed"
			case "duplicate":
				c.Files = append(c.Files, c.Files[0])
			case "revision":
				c.ObservedRevision = "r2"
			case "partial":
				c.Outcome = "unavailable"
			case "unknown":
				c.Outcome = "assumed_complete"
			case "capacity":
				c.Files[0].Content = strings.Repeat("x", RuntimeValidationGenerationContextMaxBytes+1)
				c.Files[0].ContentHash = RuntimeValidationInputContentHash(c.Files[0].Content)
			case "path":
				c.Files[0].TargetRef = "../other/source"
			}
			if c.Validate("r1") == nil {
				t.Fatal("invalid context accepted")
			}
		})
	}
	for _, outcome := range []string{"unavailable", "over_capacity"} {
		c := base.Clone()
		c.Outcome, c.Files = RuntimeValidationGenerationContextOutcome(outcome), nil
		if err := c.Validate("r1"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidationGenerationContextEntersCatalogIdentityAndRawClone(t *testing.T) {
	base := RuntimeOperationAdmissibility{ValidationSurfaces: []RuntimeValidationSurface{{ID: "surface"}}}
	old := StableRuntimeOperationAdmissibilitySnapshotID(base)
	file := RuntimeValidationInputReviewFile{TargetRef: "source", Content: "original", ContentHash: RuntimeValidationInputContentHash("original")}
	base.ValidationSurfaces[0].GenerationContext = &RuntimeValidationGenerationContext{Version: RuntimeValidationGenerationContextVersion, ContractRef: "owner/v1", ObservedRevision: "revision", Outcome: "complete", Files: []RuntimeValidationInputReviewFile{file}}
	first := StableRuntimeOperationAdmissibilitySnapshotID(base)
	if first == old {
		t.Fatal("context omitted from catalog identity")
	}
	cloned := cloneRuntimeValidationSurfaceRaw(base.ValidationSurfaces[0])
	cloned.GenerationContext.Files[0].Content = "new actual source"
	cloned.GenerationContext.Files[0].ContentHash = RuntimeValidationInputContentHash(cloned.GenerationContext.Files[0].Content)
	if base.ValidationSurfaces[0].GenerationContext.Files[0].Content != "original" {
		t.Fatal("raw clone aliases source")
	}
	base.ValidationSurfaces[0] = cloned
	if StableRuntimeOperationAdmissibilitySnapshotID(base) == first {
		t.Fatal("changed context reused catalog identity")
	}
}

func TestGenerationContextCannotEscapeOwnerRevisionOrPreparation(t *testing.T) {
	manifest, _, _ := semanticMissingOwnerFactAcquisitionFixture(t)
	var catalog RuntimeOperationAdmissibility
	for _, item := range manifest.OwnerCatalogs {
		if item.OwnerModuleID == RuntimeModuleShell {
			catalog = item.Clone()
		}
	}
	if len(catalog.ValidationSurfaces) == 0 {
		t.Fatal("fixture has no preparation owner")
	}
	catalog.ValidationSurfaces[0].GenerationContext = &RuntimeValidationGenerationContext{
		Version: RuntimeValidationGenerationContextVersion, ContractRef: "owner/selection/v1",
		ObservedRevision: catalog.ObservedRevision, Outcome: RuntimeValidationGenerationContextComplete,
		Files: []RuntimeValidationInputReviewFile{{TargetRef: "implementation.txt", Content: "actual bytes", ContentHash: RuntimeValidationInputContentHash("actual bytes")}},
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"revision", "bytes", "no preparation", "invented check source"} {
		t.Run(scenario, func(t *testing.T) {
			bad := catalog.Clone()
			surface := &bad.ValidationSurfaces[0]
			switch scenario {
			case "revision":
				surface.GenerationContext.ObservedRevision += "-drift"
			case "bytes":
				surface.GenerationContext.Files[0].Content += "modified"
			case "no preparation":
				surface.ProofRequirementPreparations = nil
			case "invented check source":
				p := &surface.ProofRequirementPreparations[0]
				source := RuntimeSourceObligationCheck{InventoryHash: "invented", ObligationID: "other", RequirementID: "r", VerificationID: "v", Criterion: "different criterion", Declaration: "unrelated"}
				source.CheckRef = StableRuntimeSourceObligationCheckRef(p.DefinitionRevision, p.CoverageWorkItemID, source)
				p.SourceObligationChecks = []RuntimeSourceObligationCheck{source}
			}
			// Rehashing caller-controlled metadata cannot cure scope or bytes drift.
			bad.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(bad)
			if bad.Validate() == nil {
				t.Fatal("invalid observation admitted")
			}
		})
	}
}

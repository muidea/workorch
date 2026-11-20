package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPreparationContentContractIsOptionalBoundedAndHashBound(t *testing.T) {
	catalog := RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: RuntimeModuleShell, ValidationSurfaces: []RuntimeValidationSurface{runtimeExactProofClonePreparingSurfaceFixture("recipe").Normalized()}}
	legacy, _ := json.Marshal(catalog)
	legacyHash := StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if strings.Contains(string(legacy), "ContentContractRef") || strings.Contains(string(legacy), "PreparationContentContracts") {
		t.Fatal("absent content contract changed legacy JSON")
	}
	surface := &catalog.ValidationSurfaces[0]
	p := &surface.ProofRequirementPreparations[0]
	p.ContentContractRef = "owner/input/v1"
	surface.PreparationContentContracts = []RuntimeValidationContentContract{{
		Ref:          "owner/input/v1",
		Requirements: "exact owner-authored syntax",
	}}
	if err := surface.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, ok := surface.PreparationContentRequirementsFor("owner/input/v1"); !ok || got != "exact owner-authored syntax" {
		t.Fatalf("surface failed to resolve deduplicated content contract: ok=%v got=%q", ok, got)
	}
	hash := StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if hash == legacyHash {
		t.Fatal("content contract not sealed in catalog identity")
	}
	copy := catalog.Normalized()
	copy.ValidationSurfaces[0].PreparationContentContracts[0].Requirements += " changed"
	if StableRuntimeOperationAdmissibilitySnapshotID(copy) == hash || surface.PreparationContentContracts[0].Requirements != "exact owner-authored syntax" {
		t.Fatal("copy aliased or omitted content requirements")
	}
	missingContractSurface := surface.Clone()
	missingContractSurface.PreparationContentContracts = nil
	if missingContractSurface.Validate() == nil {
		t.Fatal("surface with missing content contract ref was accepted")
	}
	orphanContractSurface := surface.Clone()
	orphanContractSurface.PreparationContentContracts = append(orphanContractSurface.PreparationContentContracts, RuntimeValidationContentContract{
		Ref:          "owner/input/unused",
		Requirements: "unused requirements",
	})
	if orphanContractSurface.Validate() == nil {
		t.Fatal("surface with unreferenced content contract was accepted")
	}
	for _, contents := range [][2]string{{"", "unbound"}, {"ref", ""}, {"ref", strings.Repeat("x", 4097)}, {"ref", "multiline\ncontract"}, {"ref", string([]byte{0xff})}} {
		bad := RuntimeValidationContentContract{Ref: contents[0], Requirements: contents[1]}
		if bad.Validate() == nil {
			t.Fatal("partial or unbounded content contract accepted")
		}
	}
	var restored RuntimeOperationAdmissibility
	if err := json.Unmarshal(legacy, &restored); err != nil || StableRuntimeOperationAdmissibilitySnapshotID(restored) != legacyHash {
		t.Fatal("legacy catalog changed across reload")
	}
}

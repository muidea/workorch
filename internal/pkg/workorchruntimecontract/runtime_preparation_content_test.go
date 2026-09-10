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
	if strings.Contains(string(legacy), "ContentContractRef") || strings.Contains(string(legacy), "ContentRequirements") {
		t.Fatal("absent content contract changed legacy JSON")
	}
	p := &catalog.ValidationSurfaces[0].ProofRequirementPreparations[0]
	p.ContentContractRef, p.ContentRequirements = "owner/input/v1", "exact owner-authored syntax"
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	hash := StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if hash == legacyHash {
		t.Fatal("content contract not sealed in catalog identity")
	}
	copy := catalog.Normalized()
	copy.ValidationSurfaces[0].ProofRequirementPreparations[0].ContentRequirements += " changed"
	if StableRuntimeOperationAdmissibilitySnapshotID(copy) == hash || p.ContentRequirements != "exact owner-authored syntax" {
		t.Fatal("copy aliased or omitted content requirements")
	}
	for _, contents := range [][2]string{{"", "unbound"}, {"ref", ""}, {"ref", strings.Repeat("x", 4097)}, {"ref", "multiline\ncontract"}, {"ref", string([]byte{0xff})}} {
		bad := p.Clone()
		bad.ContentContractRef, bad.ContentRequirements = contents[0], contents[1]
		if bad.Validate() == nil {
			t.Fatal("partial or unbounded content contract accepted")
		}
	}
	var restored RuntimeOperationAdmissibility
	if err := json.Unmarshal(legacy, &restored); err != nil || StableRuntimeOperationAdmissibilitySnapshotID(restored) != legacyHash {
		t.Fatal("legacy catalog changed across reload")
	}
}

package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func runtimeOwnerCatalogSurfaceReuseFixture(preparing bool) RuntimeOperationAdmissibility {
	catalog := RuntimeOperationAdmissibility{
		PrerequisiteID: "catalog", OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ObservedRevision: "shell-policy-v1",
	}
	for _, id := range []string{"recipe-b", "recipe-a"} {
		surface := runtimeExactProofCloneReadySurfaceFixture(id)
		if preparing {
			surface = runtimeExactProofClonePreparingSurfaceFixture(id)
		}
		catalog.ValidationSurfaces = append(catalog.ValidationSurfaces, surface)
	}
	catalog.ValidationRecipeRefs = RuntimeSelectableValidationRecipeRefs(catalog.ValidationSurfaces)
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	return catalog
}

func TestRuntimeOwnerCatalogSurfaceReusePreservesSortedDetachedProofs(t *testing.T) {
	for _, preparing := range []bool{false, true} {
		t.Run(fmt.Sprint(preparing), func(t *testing.T) {
			original := runtimeOwnerCatalogSurfaceReuseFixture(preparing)
			assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{original})
			before, _ := json.Marshal(original)
			value, err := validateAndNormalizeRuntimeOperationAdmissibility(original)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(value, original.Normalized()) || value.ValidationSurfaces[0].ID != "recipe-a" {
				t.Fatal("复用 surface 改变规范化排序或完整产物")
			}
			for index := range value.ValidationSurfaces {
				surface := &value.ValidationSurfaces[index]
				if preparing {
					surface.ProofRequirementPreparations[0].CheckAttestations[0].NativeCheckRef = "changed"
					surface.ProofRequirementPreparations[0].ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = "changed"
					surface.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = "changed"
				} else {
					surface.CheckRefs[0] = "changed"
					surface.ProofRequirementBindings[0].RequiredCheckRefs[0] = "changed"
					surface.ProofRequirementBindings[0].CheckAttestations[0].NativeCheckRef = "changed"
				}
			}
			after, _ := json.Marshal(original)
			if string(before) != string(after) {
				t.Fatal("复用 surface 共享原嵌套 proof 或 constraint")
			}
			if _, err := validateAndNormalizeRuntimeOperationAdmissibility(value); err == nil {
				t.Fatal("后续调用未重新拒绝变更 proof")
			}
			body, _ := json.Marshal(original)
			var reloaded RuntimeOperationAdmissibility
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{reloaded})
		})
	}
	manifest, _, _ := semanticInputReviewV2Fixture(t)
	assertRuntimeOwnerCatalogValidationReuse(t, manifest.OwnerCatalogs)
	for _, raw := range manifest.OwnerCatalogs {
		if raw.OwnerModuleID != RuntimeModuleShell {
			continue
		}
		before, _ := json.Marshal(raw)
		value, err := validateAndNormalizeRuntimeOperationAdmissibility(raw)
		if err != nil {
			t.Fatal(err)
		}
		value.ValidationSurfaces[0].ProofRequirementBindings[0].InputReview.Files[0].ContentHash = "changed"
		after, _ := json.Marshal(raw)
		if string(before) != string(after) {
			t.Fatal("复用 surface 共享 V2 原输入读取前像")
		}
		assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{value})
		if _, err := validateAndNormalizeRuntimeOperationAdmissibility(value); err == nil {
			t.Fatal("复用 surface 掩盖后续 V2 原读取前像漂移")
		}
	}
}

func TestRuntimeOwnerCatalogSurfaceReuseDetachesUnavailableDiagnostic(t *testing.T) {
	requirement := runtimeValidationProofRequirementForTest(t, "definition", RuntimeCoverageItem{}, "", "recipe", nil)
	surface := bindRuntimeValidationSurfaceForTest(runtimeExactProofCloneReadySurfaceFixture("recipe"), requirement)
	surface.ProofRequirementBindings, surface.CheckRefs = nil, nil
	surface.ProofRequirementUnavailabilities = []RuntimeValidationProofUnavailability{{
		Requirement: requirement, ValidationRecipeID: "recipe",
		OriginalDiagnostic: &RuntimeOriginalDiagnostic{
			SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleShell,
			ObservedRevision: "shell-policy-v1", OriginalClass: "validation_readiness", OriginalCode: "owner_input_invalid",
			OriginalMessage: "  exact cause\r\n原文  ", Signature: "owner-input/cwi", TargetRefs: []string{"checks/input"},
		},
	}}
	catalog := runtimeOwnerCatalogSurfaceReuseFixture(false)
	catalog.ValidationSurfaces, catalog.ValidationRecipeRefs = []RuntimeValidationSurface{surface}, nil
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{catalog})
	before, _ := json.Marshal(catalog)
	value, err := validateAndNormalizeRuntimeOperationAdmissibility(catalog)
	if err != nil {
		t.Fatal(err)
	}
	unavailable := &value.ValidationSurfaces[0].ProofRequirementUnavailabilities[0]
	if unavailable.OriginalDiagnostic.OriginalMessage != "  exact cause\r\n原文  " {
		t.Fatal("复用 surface 改写原 diagnostic bytes")
	}
	unavailable.OriginalDiagnostic.TargetRefs[0] = "changed"
	unavailable.Requirement.RequiredCheckRefs[0] = "changed"
	after, _ := json.Marshal(catalog)
	if string(before) != string(after) {
		t.Fatal("复用 surface 共享原 unavailable requirement 或 diagnostic")
	}
	assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{value})
}

func FuzzRuntimeOwnerCatalogSurfaceReuseNested(f *testing.F) {
	for selector := range uint8(8) {
		f.Add(selector, "", false)
		f.Add(selector, " changed\r\n原文 ", true)
	}
	f.Fuzz(func(t *testing.T, selector uint8, value string, preparing bool) {
		catalog := runtimeOwnerCatalogSurfaceReuseFixture(preparing)
		switch selector % 8 {
		case 0:
			catalog.ValidationSurfaces[0].ID = value
		case 1:
			catalog.ValidationSurfaces[0].OwnerModuleID = value
		case 2:
			catalog.ValidationSurfaces = append(catalog.ValidationSurfaces, catalog.ValidationSurfaces[0])
		case 3:
			catalog.ValidationSurfaces[0].CheckRefs = append(catalog.ValidationSurfaces[0].CheckRefs, value)
		case 4:
			catalog.ValidationSurfaces[0].InputSchemaHash = value
		case 5:
			catalog.ObservedRevision = value
		case 6:
			catalog.SnapshotID = value
		case 7:
			catalog.ValidationRecipeRefs = append(catalog.ValidationRecipeRefs, value)
		}
		if selector&8 != 0 {
			catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		}
		assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{catalog})
	})
}

func BenchmarkRuntimeOwnerCatalogSurfaceReuse(b *testing.B) {
	for _, preparing := range []bool{false, true} {
		catalog := runtimeOwnerCatalogSurfaceReuseFixture(preparing)
		b.Run(fmt.Sprint(preparing), func(b *testing.B) {
			for _, reference := range []bool{false, true} {
				name := "reused"
				if reference {
					name = "reference"
				}
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var err error
						if reference {
							err = referenceRuntimeOperationAdmissibilityValidate(catalog)
						} else {
							_, err = validateAndNormalizeRuntimeOperationAdmissibility(catalog)
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

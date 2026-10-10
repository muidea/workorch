package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func referenceRuntimeSemanticOwnerCatalogCollection(catalogs []RuntimeOperationAdmissibility) ([]RuntimeOperationAdmissibility, error) {
	session := newRuntimeSemanticValidationSession()
	normalized := make([]RuntimeOperationAdmissibility, 0, len(catalogs))
	prerequisites, owners := map[string]bool{}, map[string]bool{}
	for _, raw := range catalogs {
		key := strings.TrimSpace(raw.SnapshotID)
		var value RuntimeOperationAdmissibility
		found := false
		for _, entry := range session.catalogs[key] {
			if reflect.DeepEqual(entry.raw, raw) {
				value, found = entry.normalized, true
				break
			}
		}
		if !found {
			if err := referenceRuntimeOperationAdmissibilityValidate(raw); err != nil {
				return nil, fmt.Errorf("runtime semantic owner catalogs: %w", err)
			}
			value = raw.Normalized()
			session.catalogs[key] = append(session.catalogs[key], runtimeSemanticCatalogValidationEntry{raw: raw, normalized: value})
		}
		if prerequisites[value.PrerequisiteID] {
			return nil, fmt.Errorf("runtime semantic owner catalogs: runtime operation admissibility duplicates prerequisite %q", value.PrerequisiteID)
		}
		prerequisites[value.PrerequisiteID] = true
		if owners[value.OwnerModuleID] {
			return nil, fmt.Errorf("runtime semantic owner catalogs duplicate owner %q", value.OwnerModuleID)
		}
		owners[value.OwnerModuleID] = true
		normalized = append(normalized, value)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].PrerequisiteID != normalized[j].PrerequisiteID {
			return normalized[i].PrerequisiteID < normalized[j].PrerequisiteID
		}
		return normalized[i].OwnerModuleID < normalized[j].OwnerModuleID
	})
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

func assertRuntimeOwnerCatalogValidationReuse(t *testing.T, catalogs []RuntimeOperationAdmissibility) {
	t.Helper()
	before, err := json.Marshal(catalogs)
	if err != nil {
		t.Fatal(err)
	}
	want, wantErr := referenceRuntimeSemanticOwnerCatalogCollection(catalogs)
	for range 2 {
		got, gotErr := validateRuntimeSemanticOwnerCatalogCollectionInSession(catalogs, newRuntimeSemanticValidationSession())
		if (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) {
			t.Fatalf("目录产物／接受不等价: got=%v want=%v", gotErr, wantErr)
		}
		if gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Fatalf("目录拒绝原因改变: got=%v want=%v", gotErr, wantErr)
		}
		for _, raw := range catalogs {
			value, actualErr := validateAndNormalizeRuntimeOperationAdmissibility(raw)
			expectedErr := referenceRuntimeOperationAdmissibilityValidate(raw)
			publicErr := raw.Validate()
			if (actualErr == nil) != (expectedErr == nil) || (publicErr == nil) != (expectedErr == nil) {
				t.Fatalf("公开／私有目录门禁改变: got=%v public=%v want=%v", actualErr, publicErr, expectedErr)
			}
			if actualErr != nil {
				if actualErr.Error() != expectedErr.Error() || publicErr.Error() != expectedErr.Error() || !reflect.DeepEqual(value, RuntimeOperationAdmissibility{}) {
					t.Fatal("失败返回产物或原拒绝原因改变")
				}
			} else if !reflect.DeepEqual(value, raw.Normalized()) {
				t.Fatal("完整校验返回正文不是原规范化产物")
			}
		}
	}
	after, _ := json.Marshal(catalogs)
	if string(before) != string(after) {
		t.Fatal("校验改写原目录 bytes")
	}
}

func TestRuntimeOwnerCatalogValidationReuseMatchesIndependentReference(t *testing.T) {
	manifest, _, _, _ := semanticIndependentProofSetFixture(t)
	var original RuntimeOperationAdmissibility
	for _, catalog := range manifest.OwnerCatalogs {
		if catalog.OwnerModuleID == RuntimeModuleShell {
			original = catalog.Clone()
		}
	}
	if original.OwnerModuleID == "" {
		t.Fatal("fixture 缺少带逐 CWI proof 的 owner 目录")
	}
	original.ValidationRecipes = []RuntimeValidationRecipe{{ID: original.ValidationSurfaces[0].ID, Command: "owner-runner", Args: []string{"first", "second"}, TimeoutMillis: 1000}}
	original.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(original)
	for _, scenario := range []string{"valid", "normalized_input", "identity", "owner", "kind", "snapshot", "duplicate_variant", "duplicate_surface", "duplicate_check", "proof_identity", "surface_owner", "recipe_refs", "policy_owner", "recipe_raw", "recipe_timeout", "duplicate_recipe"} {
		t.Run(scenario, func(t *testing.T) {
			value := original.Clone()
			switch scenario {
			case "normalized_input":
				value.PrerequisiteID = " " + value.PrerequisiteID + " "
				value.TargetRefs = append(value.TargetRefs, value.TargetRefs...)
			case "identity":
				value.PrerequisiteID = ""
			case "owner":
				value.OwnerModuleID = "unowned"
			case "kind":
				value.OperationKinds = []string{"unowned"}
			case "snapshot":
				value.SnapshotID = "forged"
			case "duplicate_variant":
				value.OperationVariants = []RuntimeOperationAdmissibilityVariant{{Kind: RuntimeBlockKindManagedValidationSession}, {Kind: RuntimeBlockKindManagedValidationSession}}
			case "duplicate_surface":
				value.ValidationSurfaces = append(value.ValidationSurfaces, value.ValidationSurfaces[0])
			case "duplicate_check":
				value.ValidationSurfaces[0].CheckRefs = append(value.ValidationSurfaces[0].CheckRefs, value.ValidationSurfaces[0].CheckRefs[0])
			case "proof_identity":
				value.ValidationSurfaces[0].ProofRequirementBindings[0].RequirementHash = ""
			case "surface_owner":
				value.ValidationSurfaces[0].OwnerModuleID = "unowned"
			case "recipe_refs":
				value.ValidationRecipeRefs = []string{"unsealed"}
			case "policy_owner":
				value.OperationPolicies[0].OwnerModuleID = "unowned"
			case "recipe_raw":
				value.ValidationRecipes[0].ID = " " + value.ValidationRecipes[0].ID
			case "recipe_timeout":
				value.ValidationRecipes[0].TimeoutMillis = 1
			case "duplicate_recipe":
				value.ValidationRecipes = append(value.ValidationRecipes, value.ValidationRecipes[0])
			}
			if scenario != "snapshot" {
				value.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(value)
			}
			err := value.Validate()
			if (err == nil) != (scenario == "valid" || scenario == "normalized_input") {
				t.Fatalf("独立场景预期改变: %s: %v", scenario, err)
			}
			assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{value})
			data, _ := json.Marshal(value)
			var reloaded RuntimeOperationAdmissibility
			if err := json.Unmarshal(data, &reloaded); err != nil {
				t.Fatal(err)
			}
			assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{reloaded})
		})
	}
	assertRuntimeOwnerCatalogValidationReuse(t, nil)
	assertRuntimeOwnerCatalogValidationReuse(t, []RuntimeOperationAdmissibility{})
	assertRuntimeOwnerCatalogValidationReuse(t, manifest.OwnerCatalogs)
	duplicate := append([]RuntimeOperationAdmissibility(nil), manifest.OwnerCatalogs...)
	duplicate = append(duplicate, duplicate[0])
	assertRuntimeOwnerCatalogValidationReuse(t, duplicate)
	if _, err := validateRuntimeSemanticOwnerCatalogCollectionInSession(duplicate, newRuntimeSemanticValidationSession()); err == nil || !strings.Contains(err.Error(), "duplicates prerequisite") {
		t.Fatalf("重复身份未拒绝: %v", err)
	}

	before, _ := json.Marshal(original)
	value, err := validateAndNormalizeRuntimeOperationAdmissibility(original)
	if err != nil {
		t.Fatal(err)
	}
	value.OperationKinds[0] = "changed"
	value.ValidationSurfaces[0].CheckRefs[0] = "changed"
	value.ValidationSurfaces[0].ProofRequirementBindings[0].CheckAttestations[0].NativeCheckRef = "changed"
	value.OperationPolicies[0].ID = "changed"
	value.ValidationRecipes[0].Args[0] = "changed"
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("返回的目录与原嵌套 proof／policy 共享可变值")
	}
	original.TargetRefs = append(original.TargetRefs, "unsealed-after-success")
	if _, err := validateAndNormalizeRuntimeOperationAdmissibility(original); err == nil {
		t.Fatal("成功后新调用未重新校验完整 snapshot")
	}
}

func FuzzRuntimeOwnerCatalogValidationReuse(f *testing.F) {
	ledger := semanticBindingIndexedMultiBatchFixture(f, 2)
	data, _ := json.Marshal(ledger.Manifest.OwnerCatalogs)
	f.Add(data)
	f.Add([]byte("null"))
	f.Add([]byte("[]"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var catalogs []RuntimeOperationAdmissibility
		if err := json.Unmarshal(data, &catalogs); err != nil {
			return
		}
		assertRuntimeOwnerCatalogValidationReuse(t, catalogs)
	})
}

func BenchmarkRuntimeOwnerCatalogValidationReuse(b *testing.B) {
	ledger := semanticBindingIndexedMultiBatchFixture(b, 8)
	nested := semanticTestOwnerCatalog("nested-catalog", RuntimeModuleShell, []string{RuntimeBlockKindManagedValidationSession})
	for idx := range 8 {
		id := fmt.Sprintf("recipe-%02d", idx)
		nested.ValidationRecipes = append(nested.ValidationRecipes, RuntimeValidationRecipe{ID: id, Command: "owner-runner", Args: []string{"first", "second"}, TimeoutMillis: 1000})
		nested.ValidationSurfaces = append(nested.ValidationSurfaces, RuntimeValidationSurface{
			ID: id, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
			Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "input-schema", InputBindingRef: "input-binding",
			ProofApplicability: RuntimeValidationProofAuxiliary, EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			InputTargetOwnerModuleID: RuntimeModuleFile, InputTargetRefs: []string{"input-a", "input-b"},
		})
	}
	nested = nested.Normalized()
	nested.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(nested)
	if err := nested.Validate(); err != nil {
		b.Fatal(err)
	}
	for _, fixture := range []struct {
		name     string
		catalogs []RuntimeOperationAdmissibility
	}{{"file", ledger.Manifest.OwnerCatalogs}, {"nested_shell", []RuntimeOperationAdmissibility{nested}}} {
		b.Run(fixture.name, func(b *testing.B) {
			for _, test := range []struct {
				name     string
				validate func([]RuntimeOperationAdmissibility) ([]RuntimeOperationAdmissibility, error)
			}{
				{"reference", referenceRuntimeSemanticOwnerCatalogCollection},
				{"reused", func(values []RuntimeOperationAdmissibility) ([]RuntimeOperationAdmissibility, error) {
					return validateRuntimeSemanticOwnerCatalogCollectionInSession(values, newRuntimeSemanticValidationSession())
				}},
			} {
				b.Run(test.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if _, err := test.validate(fixture.catalogs); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// 保留优化前集合门禁、独立规范化与索引建立，不通过当前复用 helper 构造 oracle。
func referenceSemanticProofCatalogValues(catalogs []RuntimeOperationAdmissibility) (runtimeSemanticProofCatalogs, error) {
	if err := validateRuntimeSemanticOwnerCatalogCollection(catalogs); err != nil {
		return runtimeSemanticProofCatalogs{}, err
	}
	validated := runtimeSemanticProofCatalogs{
		values:  NormalizeRuntimeOperationAdmissibilities(catalogs),
		byOwner: make(map[string]RuntimeOperationAdmissibility, len(catalogs)),
	}
	for _, value := range validated.values {
		validated.byOwner[value.OwnerModuleID] = value
	}
	return validated, nil
}

func semanticProofCatalogValuesFixture(t testing.TB) []RuntimeOperationAdmissibility {
	t.Helper()
	ledger := semanticBindingIndexedMultiBatchFixture(t, 2)
	catalogs := CloneRuntimeOperationAdmissibilities(ledger.Manifest.OwnerCatalogs)
	surface := validationSurfaceIdentityProjectionFixture(t)
	catalog := semanticTestOwnerCatalog("proof-values-shell", RuntimeModuleShell, []string{RuntimeBlockKindManagedValidationSession})
	catalog.ValidationSurfaces = []RuntimeValidationSurface{surface}
	catalog.ObservedRevision = surface.ProspectiveTargetSatisfactionConstraint.CatalogObservedRevision
	catalog.ValidationRecipeRefs = RuntimeSelectableValidationRecipeRefs(catalog.ValidationSurfaces)
	catalog.OperationPolicies = []RuntimeOperationPolicyFact{{
		ID: "proof-values-policy", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		EffectScope: surface.EffectScope, IdempotencyClass: surface.IdempotencyClass,
	}}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	catalogs = append(catalogs, catalog)
	if err := validateRuntimeSemanticOwnerCatalogCollection(catalogs); err != nil {
		t.Fatal(err)
	}
	return catalogs
}

func assertSemanticProofCatalogValuesEquivalent(t testing.TB, catalogs []RuntimeOperationAdmissibility) {
	t.Helper()
	before, err := json.Marshal(catalogs)
	if err != nil {
		t.Fatal(err)
	}
	want, wantErr := referenceSemanticProofCatalogValues(catalogs)
	for range 2 {
		got, gotErr := validateRuntimeSemanticProofCatalogs(catalogs)
		if !reflect.DeepEqual(got, want) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
			t.Fatalf("proof catalog 原值/首个错误改变: got=%v want=%v", gotErr, wantErr)
		}
	}
	after, err := json.Marshal(catalogs)
	if err != nil || string(before) != string(after) {
		t.Fatal("proof catalog 校验修改原输入")
	}
}

func TestSemanticProofCatalogValuesPreserveCompleteGateAndErrorOrder(t *testing.T) {
	original := semanticProofCatalogValuesFixture(t)
	for _, scenario := range []string{"valid", "nil", "empty", "whitespace", "duplicate_prerequisite", "duplicate_owner", "duplicate_owner_then_invalid", "duplicate_prerequisite_then_invalid", "snapshot", "surface_duplicate", "nested_proof"} {
		t.Run(scenario, func(t *testing.T) {
			catalogs := CloneRuntimeOperationAdmissibilities(original)
			switch scenario {
			case "nil":
				catalogs = nil
			case "empty":
				catalogs = []RuntimeOperationAdmissibility{}
			case "whitespace":
				catalogs[0].OwnerModuleID = " " + catalogs[0].OwnerModuleID + " "
			case "duplicate_prerequisite", "duplicate_prerequisite_then_invalid":
				catalogs = append(catalogs, catalogs[0].Clone())
			case "duplicate_owner", "duplicate_owner_then_invalid":
				duplicate := catalogs[0].Clone()
				duplicate.PrerequisiteID += "-other"
				duplicate.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(duplicate)
				catalogs = append(catalogs, duplicate)
			case "snapshot":
				catalogs[0].SnapshotID = "changed"
			case "surface_duplicate", "nested_proof":
				for index := range catalogs {
					if len(catalogs[index].ValidationSurfaces) == 0 {
						continue
					}
					if scenario == "surface_duplicate" {
						catalogs[index].ValidationSurfaces = append(catalogs[index].ValidationSurfaces, catalogs[index].ValidationSurfaces[0].Clone())
					} else {
						catalogs[index].ValidationSurfaces[0].ProofRequirementPreparations[0].RequirementHash = "changed"
					}
					break
				}
			}
			if scenario == "duplicate_owner_then_invalid" || scenario == "duplicate_prerequisite_then_invalid" {
				invalid := original[0].Clone()
				invalid.SnapshotID = "invalid"
				catalogs = append(catalogs, invalid)
			}
			assertSemanticProofCatalogValuesEquivalent(t, catalogs)
			data, err := json.Marshal(catalogs)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded []RuntimeOperationAdmissibility
			if err := json.Unmarshal(data, &reloaded); err != nil {
				t.Fatal(err)
			}
			assertSemanticProofCatalogValuesEquivalent(t, reloaded)
		})
	}
	got, err := validateRuntimeSemanticProofCatalogs(original)
	if err != nil {
		t.Fatal(err)
	}
	for index := range got.values {
		if len(got.values[index].ValidationSurfaces) != 0 {
			got.values[index].ValidationSurfaces[0].ProofRequirementPreparations[0].RequirementHash = "caller-change"
			break
		}
	}
	assertSemanticProofCatalogValuesEquivalent(t, original)
	original[0].ObservedRevision = "later-drift"
	if _, err := validateRuntimeSemanticProofCatalogs(original); err == nil {
		t.Fatal("后续输入变化复用前次成功")
	}
}

func TestSemanticProofCatalogValuesReduceCompleteValidationAllocations(t *testing.T) {
	catalogs := semanticProofCatalogValuesFixture(t)
	current := testing.AllocsPerRun(20, func() {
		if _, err := validateRuntimeSemanticProofCatalogs(catalogs); err != nil {
			panic(err)
		}
	})
	reference := testing.AllocsPerRun(20, func() {
		if _, err := referenceSemanticProofCatalogValues(catalogs); err != nil {
			panic(err)
		}
	})
	if current >= reference {
		t.Fatalf("完整 catalog 校验未减少分配: current=%v reference=%v", current, reference)
	}
	t.Logf("allocations current=%v reference=%v", current, reference)
}

func BenchmarkSemanticProofCatalogValues(b *testing.B) {
	catalogs := semanticProofCatalogValuesFixture(b)
	for _, reference := range []bool{false, true} {
		name := "validated_values"
		if reference {
			name = "reference"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if reference {
					_, err = referenceSemanticProofCatalogValues(catalogs)
				} else {
					_, err = validateRuntimeSemanticProofCatalogs(catalogs)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func FuzzSemanticProofCatalogValuesMatchCompleteValidation(f *testing.F) {
	original := semanticProofCatalogValuesFixture(f)
	for _, seed := range []string{"", "changed", " ", "中文", "\xff"} {
		f.Add(seed, uint8(0))
	}
	f.Fuzz(func(t *testing.T, value string, mode uint8) {
		if len(value) > 512 {
			return
		}
		catalogs := CloneRuntimeOperationAdmissibilities(original)
		switch mode % 5 {
		case 0:
			catalogs[0].OwnerModuleID = value
		case 1:
			catalogs[0].PrerequisiteID = value
		case 2:
			catalogs[0].TargetRefs = append(catalogs[0].TargetRefs, value)
		case 3:
			catalogs = append(catalogs, catalogs[0].Clone())
			catalogs[len(catalogs)-1].ObservedRevision = value
		case 4:
			catalogs[0].SnapshotID = value
		}
		assertSemanticProofCatalogValuesEquivalent(t, catalogs)
	})
}

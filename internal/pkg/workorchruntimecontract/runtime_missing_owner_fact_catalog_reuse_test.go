package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func referenceMissingOwnerFactsBeforeCatalogReuse(requirements []RuntimeOwnerFactRequirement, catalogs []RuntimeOperationAdmissibility) []RuntimeOwnerFactRequirement {
	requirements = NormalizeRuntimeOwnerFactRequirements(requirements)
	catalogs = NormalizeRuntimeOperationAdmissibilities(catalogs)
	missing := make([]RuntimeOwnerFactRequirement, 0)
	for _, requirement := range requirements {
		requirement = requirement.Normalized()
		satisfied := false
		for _, catalog := range catalogs {
			catalog = catalog.Normalized()
			if catalog.OwnerModuleID == requirement.OwnerModuleID && runtimeOperationKindsContain(catalog.OperationKinds, requirement.OperationKind) && assessmentOwnerFactSatisfiedReference(requirement, catalog) {
				satisfied = true
				break
			}
		}
		if !satisfied {
			missing = append(missing, requirement)
		}
	}
	return nilIfEmpty(missing)
}

func missingOwnerFactCatalogReuseFixture() ([]RuntimeOwnerFactRequirement, []RuntimeOperationAdmissibility) {
	catalog := runtimeOwnerCatalogSurfaceReuseFixture(false)
	requirements := []RuntimeOwnerFactRequirement{{OwnerModuleID: catalog.OwnerModuleID, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}}
	for _, surface := range catalog.ValidationSurfaces {
		requirement := requirements[0]
		requirement.ValidationProofRequirementHash = surface.ProofRequirementBindings[0].RequirementHash
		requirements = append(requirements, requirement)
	}
	return requirements, []RuntimeOperationAdmissibility{catalog}
}

func assertMissingOwnerFactCatalogReuseReference(t testing.TB, requirements []RuntimeOwnerFactRequirement, catalogs []RuntimeOperationAdmissibility) {
	t.Helper()
	beforeRequirements := cloneRuntimeStateRawSlice(requirements, func(value RuntimeOwnerFactRequirement) RuntimeOwnerFactRequirement { return value })
	beforeCatalogs := cloneRuntimeStateRawSlice(catalogs, cloneRuntimeOperationAdmissibilityRaw)
	want, got := referenceMissingOwnerFactsBeforeCatalogReuse(requirements, catalogs), RuntimeMissingOwnerFactRequirements(requirements, catalogs)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("同次 catalog 复用改变 missing 集合：got=%+v want=%+v", got, want)
	}
	if len(got) != 0 {
		got[0].OwnerModuleID = "changed-return"
	}
	if !reflect.DeepEqual(requirements, beforeRequirements) || !reflect.DeepEqual(catalogs, beforeCatalogs) {
		t.Fatal("missing 查询污染原 requirement/catalog")
	}
	if !reflect.DeepEqual(RuntimeMissingOwnerFactRequirements(requirements, catalogs), want) {
		t.Fatal("返回集合污染后续查询")
	}
	body, err := json.Marshal(catalogs)
	if err != nil {
		t.Fatal(err)
	}
	var loaded []RuntimeOperationAdmissibility
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(RuntimeMissingOwnerFactRequirements(requirements, loaded), referenceMissingOwnerFactsBeforeCatalogReuse(requirements, loaded)) {
		t.Fatal("重载后 missing 查询漂移")
	}
}

func TestMissingOwnerFactCatalogReuseKeepsOriginalClassificationAndValues(t *testing.T) {
	requirements, catalogs := missingOwnerFactCatalogReuseFixture()
	if err := catalogs[0].Validate(); err != nil {
		t.Fatal(err)
	}
	if got := RuntimeMissingOwnerFactRequirements(requirements, catalogs); len(got) != 0 {
		t.Fatalf("合法 ready/direct/published/bound 声明未满足：%+v", got)
	}
	assertMissingOwnerFactCatalogReuseReference(t, nil, nil)
	assertMissingOwnerFactCatalogReuseReference(t, []RuntimeOwnerFactRequirement{}, []RuntimeOperationAdmissibility{})
	for _, preparing := range []bool{false, true} {
		assertMissingOwnerFactCatalogReuseReference(t, requirements, []RuntimeOperationAdmissibility{runtimeOwnerCatalogSurfaceReuseFixture(preparing)})
	}
	mutations := []struct {
		name   string
		mutate func([]RuntimeOwnerFactRequirement, []RuntimeOperationAdmissibility)
	}{
		{"trim", func(r []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) {
			r[0].OwnerModuleID = " " + r[0].OwnerModuleID + " "
			c[0].OwnerModuleID = " " + c[0].OwnerModuleID + "\r\n"
			c[0].ValidationSurfaces[0].ID = " " + c[0].ValidationSurfaces[0].ID + " "
			c[0].ValidationSurfaces[0].Readiness = " ready "
		}},
		{"no-prerequisite", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) { c[0].PrerequisiteID = " " }},
		{"owner", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) { c[0].OwnerModuleID = "other" }},
		{"kind", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) {
			c[0].OperationKinds = []string{"unknown"}
		}},
		{"unpublished", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) {
			c[0].ValidationRecipeRefs = nil
		}},
		{"blank-surface", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) {
			c[0].ValidationSurfaces[0].ID = " "
		}},
		{"auxiliary", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) {
			c[0].ValidationSurfaces[0].ProofApplicability = RuntimeValidationProofAuxiliary
		}},
		{"unavailable", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) {
			c[0].ValidationSurfaces[0].Readiness = RuntimeValidationSurfaceUnavailable
		}},
		{"missing-proof", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) {
			c[0].ValidationSurfaces[0].ProofRequirementBindings = nil
		}},
		{"hash-drift", func(_ []RuntimeOwnerFactRequirement, c []RuntimeOperationAdmissibility) {
			c[0].ValidationSurfaces[0].ProofRequirementBindings[0].RequirementHash = "changed"
		}},
		{"blank-requirement", func(r []RuntimeOwnerFactRequirement, _ []RuntimeOperationAdmissibility) { r[0].OwnerModuleID = " " }},
		{"unknown-fact", func(r []RuntimeOwnerFactRequirement, _ []RuntimeOperationAdmissibility) { r[0].FactKind = "unknown" }},
		{"unknown-hash", func(r []RuntimeOwnerFactRequirement, _ []RuntimeOperationAdmissibility) {
			r[0].ValidationProofRequirementHash = "unknown"
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			r, c := missingOwnerFactCatalogReuseFixture()
			mutation.mutate(r, c)
			assertMissingOwnerFactCatalogReuseReference(t, r, c)
			c = append(c, cloneRuntimeOperationAdmissibilityRaw(c[0]), catalogs[0])
			r = append(r, r[0], RuntimeOwnerFactRequirement{})
			assertMissingOwnerFactCatalogReuseReference(t, r, c)
		})
	}
	changed := cloneRuntimeOperationAdmissibilityRaw(catalogs[0])
	changed.ValidationSurfaces[0].ProofRequirementBindings[0].CheckAttestations[0].NativeCheckRef = "later-change"
	wantErr := catalogs[0].Validate()
	_ = RuntimeMissingOwnerFactRequirements(requirements, []RuntimeOperationAdmissibility{changed})
	if err := changed.Validate(); err == nil || wantErr != nil {
		t.Fatal("missing 查询替代了完整 owner proof 门禁")
	}
	assertMissingOwnerFactCatalogReuseReference(t, requirements, []RuntimeOperationAdmissibility{changed})
}

func TestMissingOwnerFactCatalogReuseRemovesOnlyRepeatedCopies(t *testing.T) {
	requirements, catalogs := missingOwnerFactCatalogReuseFixture()
	original := testing.AllocsPerRun(20, func() { _ = referenceMissingOwnerFactsBeforeCatalogReuse(requirements, catalogs) })
	current := testing.AllocsPerRun(20, func() { _ = RuntimeMissingOwnerFactRequirements(requirements, catalogs) })
	if current >= original {
		t.Fatalf("重复 catalog 分配未减少：current=%v original=%v", current, original)
	}
}

func FuzzMissingOwnerFactCatalogReuseReference(f *testing.F) {
	f.Add("blocks/shell", "ready", "direct", "recipe-b", "hash", uint8(0))
	f.Add(string([]byte{0xff, 0x80}), " ", "\x00", "中文", "", uint8(7))
	f.Fuzz(func(t *testing.T, owner, readiness, applicability, id, hash string, field uint8) {
		r, c := missingOwnerFactCatalogReuseFixture()
		switch field % 8 {
		case 0:
			c[0].OwnerModuleID, r[0].OwnerModuleID = owner, owner
		case 1:
			c[0].ValidationSurfaces[0].Readiness = RuntimeValidationSurfaceReadiness(readiness)
		case 2:
			c[0].ValidationSurfaces[0].ProofApplicability = RuntimeValidationProofApplicability(applicability)
		case 3:
			c[0].ValidationSurfaces[0].ID, c[0].ValidationRecipeRefs = id, []string{id, id, " "}
		case 4:
			c[0].ValidationSurfaces[0].ProofRequirementBindings[0].RequirementHash, r[0].ValidationProofRequirementHash = hash, hash
		case 5:
			c[0].PrerequisiteID = id
		case 6:
			c[0].OperationKinds, r[0].OperationKind = []string{id}, id
		case 7:
			r[0].FactKind = RuntimeOwnerFactKind(applicability)
		}
		c = append(c, cloneRuntimeOperationAdmissibilityRaw(c[0]))
		r = append(r, r[0])
		assertMissingOwnerFactCatalogReuseReference(t, r, c)
	})
}

func BenchmarkMissingOwnerFactCatalogReuse(b *testing.B) {
	requirements, catalogs := missingOwnerFactCatalogReuseFixture()
	for _, mode := range []string{"reference", "current"} {
		b.Run(fmt.Sprint(mode), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = referenceMissingOwnerFactsBeforeCatalogReuse(requirements, catalogs)
				} else {
					_ = RuntimeMissingOwnerFactRequirements(requirements, catalogs)
				}
			}
		})
	}
}

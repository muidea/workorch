package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSemanticProofCatalogReusePreservesPublicConstraintGates(t *testing.T) {
	manifest, _, _ := semanticMissingOwnerFactAcquisitionFixture(t)
	catalogs := CloneRuntimeOperationAdmissibilities(manifest.OwnerCatalogs)
	acquisition := manifest.ProofObligations[0].OwnerFactAcquisition
	if acquisition == nil {
		t.Fatal("夹具没有 acquisition 前像")
	}
	var dependency RuntimeOperationAdmissibility
	for _, catalog := range catalogs {
		if catalog.OwnerModuleID == acquisition.Requirement.OwnerModuleID {
			dependency = catalog.Clone()
		}
	}
	for _, test := range []struct {
		name   string
		change func(*RuntimeOperationAdmissibility, *RuntimeOwnerFactRequirement, *string)
	}{
		{"valid", func(*RuntimeOperationAdmissibility, *RuntimeOwnerFactRequirement, *string) {}},
		{"wrong_owner", func(_ *RuntimeOperationAdmissibility, requirement *RuntimeOwnerFactRequirement, _ *string) {
			requirement.OwnerModuleID = RuntimeModuleFile
		}},
		{"wrong_contract", func(_ *RuntimeOperationAdmissibility, _ *RuntimeOwnerFactRequirement, ref *string) { *ref = "missing" }},
		{"missing_constraint", func(catalog *RuntimeOperationAdmissibility, _ *RuntimeOwnerFactRequirement, _ *string) {
			for index := range catalog.ValidationSurfaces {
				catalog.ValidationSurfaces[index].ProspectiveTargetSatisfactionConstraint = nil
				catalog.ValidationSurfaces[index].ProofRequirementPreparations = nil
			}
		}},
		{"duplicate_surface", func(catalog *RuntimeOperationAdmissibility, _ *RuntimeOwnerFactRequirement, _ *string) {
			catalog.ValidationSurfaces = append(catalog.ValidationSurfaces, catalog.ValidationSurfaces[0].Clone())
		}},
		{"diagnostic_identity", func(catalog *RuntimeOperationAdmissibility, _ *RuntimeOwnerFactRequirement, _ *string) {
			catalog.SnapshotID = "wrong"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog, requirement, ref := dependency.Clone(), acquisition.Requirement, acquisition.ProspectiveSatisfactionContractRef
			test.change(&catalog, &requirement, &ref)
			before, _ := json.Marshal(catalog)
			want, wantErr := ResolveRuntimeProspectiveTargetSatisfactionConstraint(catalog, requirement, ref)
			// 与 compiler 相同：raw 集合门禁必须先成功，不允许自洽 hash 或 normalization 修复坏输入。
			err := validateRuntimeSemanticOwnerCatalogCollection([]RuntimeOperationAdmissibility{catalog})
			if err != nil {
				if wantErr == nil {
					t.Fatal("raw 门禁与公开 resolver 拒绝不一致")
				}
				return
			}
			if err = requirement.Validate(); err != nil {
				if wantErr == nil {
					t.Fatal("requirement 门禁拒绝不一致")
				}
				return
			}
			got, _, gotErr := resolveRuntimeProspectiveTargetSatisfactionConstraintNormalized(catalog.Normalized(), requirement.Normalized(), strings.TrimSpace(ref))
			if (gotErr == nil) != (wantErr == nil) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
				t.Fatalf("公开／私有 resolver 拒绝漂移: got=%v want=%v", gotErr, wantErr)
			}
			if gotErr == nil && !reflect.DeepEqual(got.Clone(), want) {
				t.Fatal("完整 all-of 产物漂移")
			}
			after, _ := json.Marshal(catalog)
			if !bytes.Equal(before, after) {
				t.Fatal("原始目录被改写")
			}
		})
	}
}

func TestSemanticManifestProofCatalogReuseKeepsEveryImmutableCWI(t *testing.T) {
	ledger := semanticBindingIndexedMultiBatchFixture(t, 6)
	manifest, seed := ledger.Manifest.Clone(), ledger.DefinitionSeed.Clone()
	sources := semanticDefinitionFocuses(seed.CoverageItems)
	if err := ValidateRuntimeSemanticPlanManifestAgainstDefinition(manifest, seed, sources); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(manifest)
	for index := range manifest.ProofObligations {
		t.Run(string(manifest.ProofObligations[index].CoverageWorkItemID), func(t *testing.T) {
			changed := manifest.Clone()
			changed.ProofObligations[index].RequireAssessment = false
			changed.Hash = StableRuntimeSemanticPlanManifestHash(changed)
			if err := changed.Validate(); err != nil {
				t.Fatalf("反例必须先满足自洽 manifest: %v", err)
			}
			if err := ValidateRuntimeSemanticPlanManifestAgainstDefinition(changed, seed, sources); err == nil {
				t.Fatal("目录复用遗漏某条独立 Definition 反证")
			}
		})
	}
	after, _ := json.Marshal(manifest)
	if !bytes.Equal(before, after) {
		t.Fatal("原 manifest 被改写")
	}
	if err := ValidateRuntimeSemanticPlanManifestAgainstDefinition(manifest, seed, sources); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticProofCatalogReuseRechecksRawCatalogsAndDetaches(t *testing.T) {
	manifest, _, _ := semanticMissingOwnerFactAcquisitionFixture(t)
	catalogs := CloneRuntimeOperationAdmissibilities(manifest.OwnerCatalogs)
	// 只用于夹具复原输入；生产 compiler 的权威源仍是独立 Definition。
	source := runtimeSemanticProofObligationDefinitionSource(manifest.ProofObligations[0])
	source.ID = "definition-source"
	source.AssessmentScope = manifest.CoverageItems[0].SourceScope
	source.AssessmentID = manifest.CoverageItems[0].SourceID
	source.AssessmentText = manifest.CoverageItems[0].TargetText
	sources := []RuntimeDefinitionFocus{source}
	before, _ := json.Marshal(catalogs)
	got, err := compileRuntimeSemanticProofObligations(manifest.DefinitionRevision, sources, manifest.CoverageItems, catalogs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, manifest.ProofObligations) {
		actual, _ := json.Marshal(got)
		expected, _ := json.Marshal(manifest.ProofObligations)
		t.Fatalf("完整 obligation 产物漂移: got=%s want=%s", actual, expected)
	}
	got[0].OwnerFactAcquisition.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = "changed"
	again, err := compileRuntimeSemanticProofObligations(manifest.DefinitionRevision, sources, manifest.CoverageItems, catalogs)
	if err != nil || !reflect.DeepEqual(again, manifest.ProofObligations) {
		t.Fatalf("返回值污染后续调用: %v", err)
	}
	after, _ := json.Marshal(catalogs)
	if !bytes.Equal(before, after) {
		t.Fatal("原始目录被改写")
	}
	var reloaded []RuntimeOperationAdmissibility
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	reloadedResult, err := compileRuntimeSemanticProofObligations(manifest.DefinitionRevision, sources, manifest.CoverageItems, reloaded)
	if err != nil || !reflect.DeepEqual(again, reloadedResult) {
		t.Fatalf("重载产物漂移: %v", err)
	}
	for _, test := range []struct {
		name   string
		change func([]RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility
	}{
		{"duplicate_owner", func(values []RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility {
			duplicate := values[0].Clone()
			duplicate.PrerequisiteID += "-other"
			duplicate.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(duplicate)
			return append(values, duplicate)
		}},
		{"duplicate_prerequisite", func(values []RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility {
			return append(values, values[0].Clone())
		}},
		{"stale_hash", func(values []RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility {
			values[0].ObservedRevision += "-changed"
			return values
		}},
		{"duplicate_surface", func(values []RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility {
			for index := range values {
				if len(values[index].ValidationSurfaces) != 0 {
					values[index].ValidationSurfaces = append(values[index].ValidationSurfaces, values[index].ValidationSurfaces[0].Clone())
					values[index].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(values[index])
					break
				}
			}
			return values
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := test.change(CloneRuntimeOperationAdmissibilities(catalogs))
			if _, err := compileRuntimeSemanticProofObligations(manifest.DefinitionRevision, sources, manifest.CoverageItems, changed); err == nil {
				t.Fatal("后续调用没有重新反证 raw 目录")
			}
		})
	}
}

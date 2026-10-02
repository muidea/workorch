package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCoverageEvidenceBindingStableHashPreservesCanonicalIdentity(t *testing.T) {
	base := testCoverageEvidenceBinding()
	extended := base.Clone()
	extended.TargetRefs = nil
	extended.TargetSelectorRef = "file/profile_declared_candidates"
	extended.TargetSelectorOwnerRef = RuntimeModuleFile
	extended.TargetSelectorSubjectRef = "deliverable"
	extended.TargetSelectorCandidateRefs = []string{"main.go", "supplement_test.go"}
	extended.ProspectiveTargetAllOfConstraintRefs = []string{"new-target-requires-owner-selector", "workspace-relative-path"}
	extended.ProspectiveTargetRefs = []string{"main.go", "supplement_test.go"}
	extended.PrerequisiteRefs = []string{"catalog/file", "catalog/shell"}
	extended.VerificationPolicy = &RuntimeVerificationPolicy{Version: RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 2, MinIndependentSources: 2, RequireBoundContext: true}
	extended.Proofs[1].ValidationCheckAttestations[0].SupplementalNativeCheckRef = "test/native/supplement"
	extended = extended.Normalized()
	extended.Hash = StableCoverageEvidenceBindingHash(extended)

	for _, test := range []struct {
		name    string
		binding CoverageEvidenceBinding
		golden  string
	}{
		{"base", base, "c8a5fda4dea752535b83a9a881bf8a1c934755cecda9ce4bb8e5c3fafee431e6"},
		{"extended", extended, "dcb8fafd68fba763c4c6f8488bad619d8bd8b0ba1fb3cfcecd65ddfaf836f577"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.binding.Hash != test.golden || StableCoverageEvidenceBindingHash(test.binding) != test.golden {
				t.Fatalf("既有封存 hash 改变: got=%s want=%s", test.binding.Hash, test.golden)
			}
			before := test.binding.Clone()
			if err := test.binding.Validate(); err != nil {
				t.Fatal(err)
			}
			if err := ValidateCoverageEvidenceBindingProjection([]CoverageEvidenceBinding{test.binding}); err != nil {
				t.Fatal(err)
			}
			aliased := test.binding.Clone()
			aliased.ID = " " + aliased.ID + " "
			aliased.Hash = "ignored caller hash"
			aliased.Proofs[0], aliased.Proofs[1] = aliased.Proofs[1], aliased.Proofs[0]
			for _, refs := range [][]string{aliased.TargetSelectorCandidateRefs, aliased.ProspectiveTargetAllOfConstraintRefs, aliased.ProspectiveTargetRefs, aliased.PrerequisiteRefs} {
				if len(refs) == 2 {
					refs[0], refs[1] = refs[1], refs[0]
				}
			}
			for index := range aliased.Proofs {
				aliased.Proofs[index].ArtifactRefs = append(aliased.Proofs[index].ArtifactRefs, " ")
			}
			if StableCoverageEvidenceBindingHash(aliased) != test.binding.Hash {
				t.Fatal("公开 hash 改变 normalization 或 Hash 字段排除语义")
			}
			if err := ValidateCoverageEvidenceBindingProjection([]CoverageEvidenceBinding{aliased}); err == nil {
				t.Fatal("等价 hash 绕过 raw-canonical projection gate")
			}
			encoded, err := json.Marshal(test.binding)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded CoverageEvidenceBinding
			if err := json.Unmarshal(encoded, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := reloaded.Validate(); err != nil || StableCoverageEvidenceBindingHash(reloaded) != test.binding.Hash {
				t.Fatalf("同版本重载改变封存 hash: %v", err)
			}
			for range 2 {
				if StableCoverageEvidenceBindingHash(test.binding) != test.binding.Hash {
					t.Fatal("重复 hash 调用改变身份")
				}
			}
			if !reflect.DeepEqual(before, test.binding) {
				t.Fatal("校验或 hash 计算修改调用者值")
			}
			reloaded.Proofs[1].ValidationCheckAttestations[0].NativeCheckRef = "changed"
			if err := reloaded.Validate(); err == nil {
				t.Fatal("先前校验成功缓存掩盖后续 body 漂移")
			}
			if test.name == "extended" {
				changed := test.binding.Clone()
				changed.Proofs[1].ValidationCheckAttestations[0].SupplementalNativeCheckRef = "other-supplement"
				if StableCoverageEvidenceBindingHash(changed) == test.golden || changed.Validate() == nil {
					t.Fatal("补充 native 身份未进入封存 hash")
				}
			}
		})
	}
}

func BenchmarkCoverageEvidenceBindingNormalize(b *testing.B) {
	binding := testCoverageEvidenceBinding()
	b.ReportAllocs()
	for b.Loop() {
		NormalizeCoverageEvidenceBindings([]CoverageEvidenceBinding{binding})
	}
}

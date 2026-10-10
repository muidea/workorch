package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

// 保留优化前的 Clone 作为独立 reference，不以新实现自证等价。
func runtimeCoverageProofCloneReference(p CoverageEvidenceProof) CoverageEvidenceProof {
	p = p.Normalized()
	p.RequiredEffectScopes = append([]RuntimeEffectScope(nil), p.RequiredEffectScopes...)
	p.AllowedOperationKinds = append([]string(nil), p.AllowedOperationKinds...)
	p.AllowedOperationVariants = append([]RuntimeOperationVariantRef(nil), p.AllowedOperationVariants...)
	p.RequiredCheckRefs = append([]string(nil), p.RequiredCheckRefs...)
	p.ValidationCheckAttestations = CloneRuntimeValidationCheckAttestationBindings(p.ValidationCheckAttestations)
	p.MetricConstraints = append([]string(nil), p.MetricConstraints...)
	p.ArtifactRefs = append([]string(nil), p.ArtifactRefs...)
	return p
}

func TestCoverageEvidenceProofCloneMatchesReferenceAndDetachesAllSlices(t *testing.T) {
	proof := testCoverageEvidenceBinding().Proofs[1]
	proof.AllowedOperationVariants = []RuntimeOperationVariantRef{{Kind: RuntimeBlockKindManagedValidationSession, VariantID: "variant"}}
	proof.MetricConstraints = []string{"metric"}
	proof.ArtifactRefs = []string{"artifact"}
	noncanonical := proof
	noncanonical.RequiredCheckRefs = []string{" check-b ", "check-a", "check-b", " "}
	noncanonical.ArtifactRefs = []string{" artifact ", "artifact"}
	for _, test := range []struct {
		name  string
		proof CoverageEvidenceProof
	}{
		{"complete", proof},
		{"noncanonical", noncanonical},
		{"nil", CoverageEvidenceProof{}},
		{"empty", CoverageEvidenceProof{AllowedOperationVariants: []RuntimeOperationVariantRef{}, RequiredCheckRefs: []string{}, ArtifactRefs: []string{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, err := json.Marshal(test.proof)
			if err != nil {
				t.Fatal(err)
			}
			want := runtimeCoverageProofCloneReference(test.proof)
			got := test.proof.Clone()
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Clone 改变规范化或 nil/empty 合同：got=%+v want=%+v", got, want)
			}
			for range 2 {
				if !reflect.DeepEqual(test.proof.Clone(), want) {
					t.Fatal("重复 Clone 改变产物")
				}
			}
			if len(got.RequiredEffectScopes) > 0 {
				got.RequiredEffectScopes[0] = RuntimeEffectWorkspaceDurable
			}
			if len(got.AllowedOperationKinds) > 0 {
				got.AllowedOperationKinds[0] = "changed"
			}
			if len(got.AllowedOperationVariants) > 0 {
				got.AllowedOperationVariants[0].VariantID = "changed"
			}
			if len(got.RequiredCheckRefs) > 0 {
				got.RequiredCheckRefs[0] = "changed"
			}
			if len(got.ValidationCheckAttestations) > 0 {
				got.ValidationCheckAttestations[0].NativeCheckRef = "changed"
			}
			if len(got.MetricConstraints) > 0 {
				got.MetricConstraints[0] = "changed"
			}
			if len(got.ArtifactRefs) > 0 {
				got.ArtifactRefs[0] = "changed"
			}
			after, err := json.Marshal(test.proof)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("Clone 的嵌套 slice 修改了来源原值")
			}
			var reloaded CoverageEvidenceProof
			if err := json.Unmarshal(before, &reloaded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(reloaded.Clone(), runtimeCoverageProofCloneReference(reloaded)) {
				t.Fatal("重载改变 Clone 合同")
			}
		})
	}
}

func runtimeCoverageBindingCloneReference(b CoverageEvidenceBinding) CoverageEvidenceBinding {
	b = b.Normalized()
	b.Proofs = append([]CoverageEvidenceProof(nil), b.Proofs...)
	for index := range b.Proofs {
		b.Proofs[index] = runtimeCoverageProofCloneReference(b.Proofs[index])
	}
	b.VerificationPolicy = cloneRuntimeVerificationPolicy(b.VerificationPolicy)
	b.TargetRefs = append([]string(nil), b.TargetRefs...)
	b.TargetSelectorCandidateRefs = append([]string(nil), b.TargetSelectorCandidateRefs...)
	b.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), b.ProspectiveTargetAllOfConstraintRefs...)
	b.ProspectiveTargetRefs = append([]string(nil), b.ProspectiveTargetRefs...)
	b.PrerequisiteRefs = append([]string(nil), b.PrerequisiteRefs...)
	return b
}

func TestCoverageEvidenceBindingCloneMatchesReferenceAndDetachesNestedValues(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	binding.VerificationPolicy = &RuntimeVerificationPolicy{Version: RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 2}
	binding.TargetSelectorCandidateRefs = []string{" main.go ", "main.go"}
	binding.ProspectiveTargetAllOfConstraintRefs = []string{"constraint"}
	binding.ProspectiveTargetRefs = []string{"prospective"}
	binding.PrerequisiteRefs = []string{"prerequisite"}
	binding.LineageAssertions = []RuntimeLineageAssertion{{Rule: RuntimeLineageAssertionRule{Ref: "rule"}}}
	for _, raw := range []CoverageEvidenceBinding{binding, {}, {Proofs: []CoverageEvidenceProof{}, TargetRefs: []string{}}} {
		before, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		got := raw.Clone()
		if !reflect.DeepEqual(got, runtimeCoverageBindingCloneReference(raw)) {
			t.Fatal("binding Clone 改变既有规范化或空值合同")
		}
		if len(got.Proofs) > 0 {
			got.Proofs[0].AllowedOperationKinds[0] = "changed"
			got.Proofs[1].ValidationCheckAttestations[0].NativeCheckRef = "changed"
		}
		if got.VerificationPolicy != nil {
			got.VerificationPolicy.MinimumEvidenceLevel = 5
		}
		if len(got.LineageAssertions) > 0 {
			got.LineageAssertions[0].Rule.Ref = "changed"
		}
		for _, refs := range [][]string{got.TargetRefs, got.TargetSelectorCandidateRefs, got.ProspectiveTargetAllOfConstraintRefs, got.ProspectiveTargetRefs, got.PrerequisiteRefs} {
			if len(refs) > 0 {
				refs[0] = "changed"
			}
		}
		after, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatal("binding Clone 修改来源嵌套原值")
		}
		var reloaded CoverageEvidenceBinding
		if err := json.Unmarshal(before, &reloaded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(reloaded.Clone(), runtimeCoverageBindingCloneReference(reloaded)) {
			t.Fatal("binding 重载改变 Clone 合同")
		}
	}
}

func BenchmarkCoverageEvidenceProofCloneReuse(b *testing.B) {
	proof := testCoverageEvidenceBinding().Proofs[1]
	for _, test := range []struct {
		name  string
		clone func(CoverageEvidenceProof) CoverageEvidenceProof
	}{{"reference", runtimeCoverageProofCloneReference}, {"single_normalization", CoverageEvidenceProof.Clone}} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				test.clone(proof)
			}
		})
	}
}

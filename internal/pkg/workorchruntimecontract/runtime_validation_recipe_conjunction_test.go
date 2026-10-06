package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// 多个 recipe 名称不能绕过单 evidence-kind 的封存合同；三阶段合取必须由实际 owner 协议证明。
func TestCoverageEvidenceBindingCannotConjoinIndependentValidationRecipes(t *testing.T) {
	baseline := testCoverageEvidenceBinding()
	var validation CoverageEvidenceProof
	for _, proof := range baseline.Proofs {
		if proof.EvidenceKind == EvidenceKindValidation {
			validation = proof.Clone()
		}
	}
	if err := validation.Validate(); err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	for _, recipes := range [][]string{{"build", "test"}, {"build", "test", "race"}} {
		t.Run(strings.Join(recipes, "+"), func(t *testing.T) {
			binding := baseline.Clone()
			binding.Proofs = nil
			for _, recipe := range recipes {
				proof := validation.Clone()
				proof.ValidationRecipeID = recipe
				if err := proof.Validate(); err != nil {
					t.Fatalf("独立 proof 应保持合法，不能以形状错误代替合取拒绝: %v", err)
				}
				binding.Proofs = append(binding.Proofs, proof)
			}
			binding.Hash = StableCoverageEvidenceBindingHash(binding)
			raw, err := json.Marshal(binding)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded CoverageEvidenceBinding
			if err := json.Unmarshal(raw, &reloaded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(binding, reloaded) {
				t.Fatal("当前 JSON 重载丢失不同 recipe 的原声明")
			}
			for _, candidate := range []CoverageEvidenceBinding{binding, binding.Clone(), reloaded} {
				if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), "duplicates evidence proof") {
					t.Fatalf("不同 recipe 绕过单 evidence-kind 合同: %v", err)
				}
			}
		})
	}
	after, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("反例修改了原 binding")
	}
}

func TestValidationObservationCannotBorrowPassedChecksAcrossRecipes(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	var proof CoverageEvidenceProof
	for _, candidate := range binding.Proofs {
		if candidate.EvidenceKind == EvidenceKindValidation {
			proof = candidate.Clone()
		}
	}
	observation := RuntimeValidationObservation{
		RecipeID:               proof.ValidationRecipeID,
		ObservationProtocolRef: proof.ValidationCheckAttestations[0].ObservationProtocolRef,
	}
	for _, attestation := range proof.ValidationCheckAttestations {
		observation.ExecutedChecks = append(observation.ExecutedChecks, attestation.NativeCheckRefs()...)
		observation.PassedChecks = append(observation.PassedChecks, attestation.NativeCheckRefs()...)
	}
	if !RuntimeValidationObservationSatisfiesCoverageProof(proof, observation) {
		t.Fatal("exact owner recipe/native-check 观察被拒绝")
	}
	before := observation.Clone()
	for _, recipe := range []string{"unexecuted-build", "unexecuted-test", "unexecuted-race"} {
		other := proof.Clone()
		other.ValidationRecipeID = recipe
		if RuntimeValidationObservationSatisfiesCoverageProof(other, observation) {
			t.Fatalf("recipe %q 借用了另一个 recipe 的 passed checks", recipe)
		}
	}
	if !reflect.DeepEqual(observation, before) {
		t.Fatal("recipe 比较改写了原 owner 观察")
	}
}

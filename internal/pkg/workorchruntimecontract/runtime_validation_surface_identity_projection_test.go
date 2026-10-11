package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func validationSurfaceIdentityProjectionFixture(t testing.TB) RuntimeValidationSurface {
	t.Helper()
	value := runtimeExactProofClonePreparingSurfaceFixture("recipe").Normalized()
	proof := value.ProofRequirementPreparations[0]
	value.ProofRequirementPreparations = nil
	for index := range 8 {
		copy := proof.Clone()
		copy.RequirementHash = fmt.Sprintf("requirement-%d", index)
		copy.CoverageWorkItemID = common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", index))
		for checkIndex := range copy.CheckAttestations {
			attestation := &copy.CheckAttestations[checkIndex]
			attestation.NativeCheckRef = fmt.Sprintf("TestNative%d_%d", index, checkIndex)
			attestation.SupplementalNativeCheckRef = fmt.Sprintf("TestSupplement%d_%d", index, checkIndex)
		}
		value.ProofRequirementPreparations = append(value.ProofRequirementPreparations, copy)
	}
	if err := value.Validate(); err != nil {
		t.Fatalf("缺少有效 multi-preparation 前像：%v", err)
	}
	return value
}

func TestValidationSurfaceIdentityProjectionKeepsFullProofAndDuplicateOrder(t *testing.T) {
	original := validationSurfaceIdentityProjectionFixture(t)
	for _, scenario := range []string{"valid", "trim", "duplicate-hash", "duplicate-cwi", "invalid-attestation-before-duplicate", "invalid-diagnostic-before-duplicate", "invalid-constraint-before-duplicate", "later-owner-drift", "cross-native", "cross-supplemental-native"} {
		t.Run(scenario, func(t *testing.T) {
			value := original.Clone()
			first, second := &value.ProofRequirementPreparations[0], &value.ProofRequirementPreparations[1]
			switch scenario {
			case "trim":
				first.RequirementHash = " " + first.RequirementHash + " "
				first.CoverageWorkItemID = " " + first.CoverageWorkItemID + " "
			case "duplicate-hash":
				second.RequirementHash = " " + first.RequirementHash + " "
			case "duplicate-cwi":
				second.CoverageWorkItemID = " " + first.CoverageWorkItemID + " "
			case "invalid-attestation-before-duplicate":
				second.RequirementHash = first.RequirementHash
				second.CheckAttestations[0].NativeCheckRef = ""
			case "invalid-diagnostic-before-duplicate":
				second.CoverageWorkItemID = first.CoverageWorkItemID
				second.OriginalDiagnostic = &RuntimeOriginalDiagnostic{OriginalMessage: "原诊断"}
			case "invalid-constraint-before-duplicate":
				second.RequirementHash = first.RequirementHash
				second.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf = nil
			case "later-owner-drift":
				second.ProspectiveTargetSatisfactionConstraint.OwnerModuleID = RuntimeModuleFile
			case "cross-native":
				second.CheckAttestations[0].NativeCheckRef = first.CheckAttestations[0].NativeCheckRef
			case "cross-supplemental-native":
				second.CheckAttestations[0].SupplementalNativeCheckRef = first.CheckAttestations[0].SupplementalNativeCheckRef
			}
			assertRuntimeValidationSurfaceSinglePass(t, value)
			body, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeValidationSurface
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			assertRuntimeValidationSurfaceSinglePass(t, reloaded)
			if scenario != "valid" && scenario != "trim" && value.Validate() == nil {
				t.Fatal("原完整 proof／duplicate／owner 反例被忽略")
			}
		})
	}
	before, _ := json.Marshal(original)
	assertRuntimeValidationSurfaceSinglePass(t, original)
	after, _ := json.Marshal(original)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("identity 投影改写原完整 proof")
	}
	original.ProofRequirementPreparations[0].CheckAttestations[0].RequiredCheckRef = ""
	if original.Validate() == nil {
		t.Fatal("后续原 proof 篡改借用先前 identity 查询成功")
	}
	assertRuntimeValidationSurfaceSinglePass(t, original)
}

func TestValidationSurfaceIdentityProjectionBindingsAndMixedProof(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		original := validationSurfaceIdentityProjectionFixture(t)
		original.Readiness = RuntimeValidationSurfaceReady
		original.ReadinessReason, original.PrerequisiteRef, original.ProspectiveSatisfactionContractRef = "", "", ""
		original.ProspectiveTargetSatisfactionConstraint = nil
		original.InputBindingRef = "input/recipe"
		count := len(original.ProofRequirementPreparations)
		if mixed {
			count /= 2
		}
		for _, preparation := range original.ProofRequirementPreparations[:count] {
			binding := RuntimeValidationProofRequirementBinding{
				RequirementHash: preparation.RequirementHash, DefinitionRevision: preparation.DefinitionRevision,
				CoverageWorkItemID: preparation.CoverageWorkItemID, DeclarationRef: preparation.DeclarationRef,
				ValidationRecipeID: preparation.ValidationRecipeID, CheckAttestations: preparation.CheckAttestations,
			}
			for _, attestation := range binding.CheckAttestations {
				binding.RequiredCheckRefs = append(binding.RequiredCheckRefs, attestation.RequiredCheckRef)
			}
			original.CheckRefs = append(original.CheckRefs, binding.RequiredCheckRefs...)
			original.ProofRequirementBindings = append(original.ProofRequirementBindings, binding)
		}
		original.CheckRefs = normalizeStrings(original.CheckRefs)
		original.ProofRequirementPreparations = original.ProofRequirementPreparations[count:]
		if err := original.Validate(); err != nil {
			t.Fatalf("缺少有效 ready/mixed 前像：%v", err)
		}
		for _, scenario := range []string{"valid", "trim", "duplicate-hash", "duplicate-cwi", "invalid-binding-before-duplicate", "cross-native", "cross-supplemental-native"} {
			t.Run(fmt.Sprintf("mixed=%v/%s", mixed, scenario), func(t *testing.T) {
				value := original.Clone()
				first := &value.ProofRequirementBindings[0]
				secondHash, secondCWI, secondChecks := &value.ProofRequirementBindings[1].RequirementHash, &value.ProofRequirementBindings[1].CoverageWorkItemID, value.ProofRequirementBindings[1].CheckAttestations
				if mixed {
					second := &value.ProofRequirementPreparations[0]
					secondHash, secondCWI, secondChecks = &second.RequirementHash, &second.CoverageWorkItemID, second.CheckAttestations
				}
				switch scenario {
				case "trim":
					first.RequirementHash = " " + first.RequirementHash + " "
					first.CoverageWorkItemID = " " + first.CoverageWorkItemID + " "
				case "duplicate-hash":
					*secondHash = " " + first.RequirementHash + " "
				case "duplicate-cwi":
					*secondCWI = " " + first.CoverageWorkItemID + " "
				case "invalid-binding-before-duplicate":
					*secondHash = first.RequirementHash
					first.CheckAttestations[0].NativeCheckRef = ""
				case "cross-native":
					secondChecks[0].NativeCheckRef = first.CheckAttestations[0].NativeCheckRef
				case "cross-supplemental-native":
					secondChecks[0].SupplementalNativeCheckRef = first.CheckAttestations[0].SupplementalNativeCheckRef
				}
				if err := referenceRuntimeValidationSurfaceValidate(value); (err == nil) != (scenario == "valid" || scenario == "trim") {
					t.Fatalf("ready/mixed 场景未命中预期：%s: %v", scenario, err)
				}
				assertRuntimeValidationSurfaceSinglePass(t, value)
				body, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var reloaded RuntimeValidationSurface
				if err := json.Unmarshal(body, &reloaded); err != nil {
					t.Fatal(err)
				}
				assertRuntimeValidationSurfaceSinglePass(t, reloaded)
			})
		}
	}
}

func TestValidationSurfaceIdentityProjectionRemovesDiscardedProofCopies(t *testing.T) {
	value := validationSurfaceIdentityProjectionFixture(t)
	original := testing.AllocsPerRun(20, func() { _ = referenceRuntimeValidationSurfaceValidate(value) })
	current := testing.AllocsPerRun(20, func() { _ = value.Validate() })
	if current >= original {
		t.Fatalf("surface 校验分配未减少：current=%v original=%v", current, original)
	}
}

func BenchmarkValidationSurfaceIdentityProjection(b *testing.B) {
	value := validationSurfaceIdentityProjectionFixture(b)
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = referenceRuntimeValidationSurfaceValidate(value)
				} else {
					_ = value.Validate()
				}
			}
		})
	}
}

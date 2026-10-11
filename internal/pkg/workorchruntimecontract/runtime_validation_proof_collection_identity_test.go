package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// 保留集合逐项完整 Validate 后再次整份 Normalized 的旧身份查询路径。
func referenceRuntimeValidationProofPreparations(values []RuntimeValidationProofPreparation) error {
	seenHash := make(map[string]struct{}, len(values))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if _, duplicate := seenHash[value.RequirementHash]; duplicate {
			return fmt.Errorf("runtime validation proof preparation %q is duplicated", value.RequirementHash)
		}
		if _, duplicate := seenCoverage[value.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime validation proof preparation coverage %q is duplicated", value.CoverageWorkItemID)
		}
		seenHash[value.RequirementHash] = struct{}{}
		seenCoverage[value.CoverageWorkItemID] = struct{}{}
	}
	return nil
}

func referenceRuntimeValidationProofRequirementBindings(values []RuntimeValidationProofRequirementBinding) error {
	seenHashes := make(map[string]struct{}, len(values))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if _, duplicate := seenHashes[value.RequirementHash]; duplicate {
			return fmt.Errorf("runtime validation proof requirement binding %q is duplicated", value.RequirementHash)
		}
		if _, duplicate := seenCoverage[value.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime validation proof requirement binding coverage %q is duplicated", value.CoverageWorkItemID)
		}
		seenHashes[value.RequirementHash] = struct{}{}
		seenCoverage[value.CoverageWorkItemID] = struct{}{}
	}
	return nil
}

func assertProofCollectionIdentityErrors(t testing.TB, got, want error) {
	t.Helper()
	if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
		t.Fatalf("proof 集合完整校验／duplicate 拒绝顺序漂移：got=%v want=%v", got, want)
	}
}

func assertProofCollectionIdentityReference(t testing.TB, preparations []RuntimeValidationProofPreparation, bindings []RuntimeValidationProofRequirementBinding) {
	t.Helper()
	beforePreparations, err := json.Marshal(preparations)
	if err != nil {
		t.Fatal(err)
	}
	beforeBindings, err := json.Marshal(bindings)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		assertProofCollectionIdentityErrors(t, ValidateRuntimeValidationProofPreparations(preparations), referenceRuntimeValidationProofPreparations(preparations))
		assertProofCollectionIdentityErrors(t, ValidateRuntimeValidationProofRequirementBindings(bindings), referenceRuntimeValidationProofRequirementBindings(bindings))
	}
	afterPreparations, _ := json.Marshal(preparations)
	afterBindings, _ := json.Marshal(bindings)
	if !reflect.DeepEqual(beforePreparations, afterPreparations) || !reflect.DeepEqual(beforeBindings, afterBindings) {
		t.Fatal("proof 集合只读身份查询改写原 bytes")
	}
	var reloadedPreparations []RuntimeValidationProofPreparation
	var reloadedBindings []RuntimeValidationProofRequirementBinding
	if err := json.Unmarshal(beforePreparations, &reloadedPreparations); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(beforeBindings, &reloadedBindings); err != nil {
		t.Fatal(err)
	}
	assertProofCollectionIdentityErrors(t, ValidateRuntimeValidationProofPreparations(reloadedPreparations), referenceRuntimeValidationProofPreparations(reloadedPreparations))
	assertProofCollectionIdentityErrors(t, ValidateRuntimeValidationProofRequirementBindings(reloadedBindings), referenceRuntimeValidationProofRequirementBindings(reloadedBindings))
}

func proofCollectionIdentityFixture() ([]RuntimeValidationProofPreparation, []RuntimeValidationProofRequirementBinding) {
	var preparations []RuntimeValidationProofPreparation
	var bindings []RuntimeValidationProofRequirementBinding
	for index := range 8 {
		preparation := runtimeExactProofClonePreparationFixture("recipe")
		binding := runtimeExactProofCloneBindingFixture("recipe")
		preparation.RequirementHash, binding.RequirementHash = fmt.Sprintf(" requirement-%d ", index), fmt.Sprintf(" requirement-%d ", index)
		preparation.CoverageWorkItemID, binding.CoverageWorkItemID = common.CoverageWorkItemID(fmt.Sprintf(" cwi-%d ", index)), common.CoverageWorkItemID(fmt.Sprintf(" cwi-%d ", index))
		preparations = append(preparations, preparation)
		bindings = append(bindings, binding)
	}
	return preparations, bindings
}

func TestProofCollectionIdentityProjectionKeepsOriginalCompleteGate(t *testing.T) {
	for _, scenario := range []string{"valid", "nil", "empty", "duplicate-hash", "duplicate-cwi", "invalid-before-duplicate", "invalid-after-earlier-duplicate", "invalid-constraint-before-duplicate", "invalid-diagnostic-before-duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			preparations, bindings := proofCollectionIdentityFixture()
			switch scenario {
			case "nil":
				preparations, bindings = nil, nil
			case "empty":
				preparations, bindings = []RuntimeValidationProofPreparation{}, []RuntimeValidationProofRequirementBinding{}
			case "duplicate-hash":
				preparations[1].RequirementHash, bindings[1].RequirementHash = preparations[0].RequirementHash, bindings[0].RequirementHash
			case "duplicate-cwi":
				preparations[1].CoverageWorkItemID, bindings[1].CoverageWorkItemID = preparations[0].CoverageWorkItemID, bindings[0].CoverageWorkItemID
			case "invalid-before-duplicate", "invalid-after-earlier-duplicate":
				preparations[1].RequirementHash, bindings[1].RequirementHash = preparations[0].RequirementHash, bindings[0].RequirementHash
				index := 1
				if scenario == "invalid-after-earlier-duplicate" {
					index = 2
				}
				preparations[index].CheckAttestations[0].NativeCheckRef, bindings[index].CheckAttestations[0].NativeCheckRef = "", ""
			case "invalid-constraint-before-duplicate":
				preparations[1].RequirementHash = preparations[0].RequirementHash
				preparations[1].ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf = nil
				bindings[1].RequirementHash = bindings[0].RequirementHash
				bindings[1].RequiredCheckRefs = append(bindings[1].RequiredCheckRefs, bindings[1].RequiredCheckRefs[0])
			case "invalid-diagnostic-before-duplicate":
				preparations[1].RequirementHash = preparations[0].RequirementHash
				preparations[1].OriginalDiagnostic = &RuntimeOriginalDiagnostic{OriginalMessage: "原诊断"}
				bindings[1].RequirementHash = bindings[0].RequirementHash
				bindings[1].DefinitionRevision = ""
			}
			valid := scenario == "valid" || scenario == "nil" || scenario == "empty"
			if (referenceRuntimeValidationProofPreparations(preparations) == nil) != valid || (referenceRuntimeValidationProofRequirementBindings(bindings) == nil) != valid {
				t.Fatal("独立完整集合场景未命中预期")
			}
			assertProofCollectionIdentityReference(t, preparations, bindings)
		})
	}
	preparations, bindings := proofCollectionIdentityFixture()
	assertProofCollectionIdentityReference(t, preparations, bindings)
	preparations[0].DefinitionRevision, bindings[0].DefinitionRevision = "", ""
	if ValidateRuntimeValidationProofPreparations(preparations) == nil || ValidateRuntimeValidationProofRequirementBindings(bindings) == nil {
		t.Fatal("后续集合正文篡改借用前次成功")
	}
	assertProofCollectionIdentityReference(t, preparations, bindings)
}

func TestProofCollectionIdentityProjectionRetainsRawDiagnosticAndInputReview(t *testing.T) {
	preparations, _ := proofCollectionIdentityFixture()
	for index := range preparations {
		constraint := preparations[index].ProspectiveTargetSatisfactionConstraint.Normalized()
		diagnostic := diagnosticFieldNormalizationFixture(16)
		diagnostic.OwnerModuleID, diagnostic.ObservedRevision = constraint.OwnerModuleID, constraint.CatalogObservedRevision
		if err := diagnostic.Validate(); err != nil {
			t.Fatalf("缺少合法原诊断前像：%v", err)
		}
		preparations[index].OriginalDiagnostic = diagnostic
	}
	_, _, _, binding := semanticInputReviewFixture(t)
	bindings := []RuntimeValidationProofRequirementBinding{binding}
	if ValidateRuntimeValidationProofPreparations(preparations) != nil || ValidateRuntimeValidationProofRequirementBindings(bindings) != nil {
		t.Fatal("合法 raw diagnostic／input review 集合被拒绝")
	}
	assertProofCollectionIdentityReference(t, preparations, bindings)
	preparations[0].OriginalDiagnostic.FieldViolations[0].Actual = "authorization: 不可持久化材料"
	bindings[0].InputReview.Files[0].ContentHash = "different"
	if ValidateRuntimeValidationProofPreparations(preparations) == nil || ValidateRuntimeValidationProofRequirementBindings(bindings) == nil {
		t.Fatal("身份查询越过原诊断安全或 input-review 完整前像门禁")
	}
	assertProofCollectionIdentityReference(t, preparations, bindings)
}

func TestProofCollectionIdentityProjectionRemovesOnlyDiscardedCopies(t *testing.T) {
	preparations, bindings := proofCollectionIdentityFixture()
	for name, funcs := range map[string][2]func(){
		"preparations": {func() { _ = referenceRuntimeValidationProofPreparations(preparations) }, func() { _ = ValidateRuntimeValidationProofPreparations(preparations) }},
		"bindings":     {func() { _ = referenceRuntimeValidationProofRequirementBindings(bindings) }, func() { _ = ValidateRuntimeValidationProofRequirementBindings(bindings) }},
	} {
		t.Run(name, func(t *testing.T) {
			original, current := testing.AllocsPerRun(20, funcs[0]), testing.AllocsPerRun(20, funcs[1])
			if current >= original {
				t.Fatalf("proof 集合身份查询分配未减少：current=%v original=%v", current, original)
			}
		})
	}
}

func FuzzProofCollectionIdentityProjection(f *testing.F) {
	f.Add(uint8(0), " requirement-0 ", " cwi-0 ")
	f.Add(uint8(1), "", "")
	f.Add(uint8(2), "原诊断\r\n\x00", string([]byte{0xff, 0x80}))
	f.Add(uint8(3), strings.Repeat("原", 32768), ".")
	f.Fuzz(func(t *testing.T, selector uint8, first, second string) {
		preparations, bindings := proofCollectionIdentityFixture()
		switch selector % 5 {
		case 0:
			preparations[1].RequirementHash, bindings[1].RequirementHash = first, first
			preparations[1].CoverageWorkItemID, bindings[1].CoverageWorkItemID = common.CoverageWorkItemID(second), common.CoverageWorkItemID(second)
		case 1:
			preparations[1].RequirementHash, bindings[1].RequirementHash = preparations[0].RequirementHash, bindings[0].RequirementHash
			preparations[1].CheckAttestations[0].NativeCheckRef, bindings[1].CheckAttestations[0].NativeCheckRef = first, first
		case 2:
			preparations[1].OriginalDiagnostic = &RuntimeOriginalDiagnostic{OriginalMessage: first, FieldViolations: []RuntimeFieldViolation{{Actual: second}}}
			bindings[1].RequiredCheckRefs = append(bindings[1].RequiredCheckRefs, second)
		case 3:
			preparations[1].ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf[0].Value = first
			bindings[1].DeclarationRef = second
		case 4:
			preparations[1].CheckAttestations[0].SupplementalNativeCheckRef, bindings[1].CheckAttestations[0].SupplementalNativeCheckRef = first, first
			preparations[1].DefinitionRevision, bindings[1].DefinitionRevision = second, second
		}
		assertProofCollectionIdentityReference(t, preparations, bindings)
	})
}

func BenchmarkProofCollectionIdentityProjection(b *testing.B) {
	preparations, bindings := proofCollectionIdentityFixture()
	for name, validate := range map[string]func() error{
		"preparations/reference": func() error { return referenceRuntimeValidationProofPreparations(preparations) },
		"preparations/current":   func() error { return ValidateRuntimeValidationProofPreparations(preparations) },
		"bindings/reference":     func() error { return referenceRuntimeValidationProofRequirementBindings(bindings) },
		"bindings/current":       func() error { return ValidateRuntimeValidationProofRequirementBindings(bindings) },
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := validate(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

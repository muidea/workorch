package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// 独立保留先 raw-clone 原诊断再执行完整 preparation 校验的旧路径。
func referenceRuntimeValidationProofPreparationValidate(p RuntimeValidationProofPreparation) error {
	rawAttestations := p.CheckAttestations
	p = p.Normalized()
	if p.RequirementHash == "" || p.DefinitionRevision == "" || p.CoverageWorkItemID == "" || p.DeclarationRef == "" ||
		p.ValidationRecipeID == "" || (p.TargetRef != "" && !runtimeProspectiveTargetRefValid(p.TargetRef)) || len(p.CheckAttestations) == 0 {
		return fmt.Errorf("runtime validation proof preparation requires exact requirement, CWI, recipe and native attestations")
	}
	if strings.Contains(p.DeclarationRef, "*") {
		return fmt.Errorf("runtime validation proof preparation declaration %q is not CWI-specific", p.DeclarationRef)
	}
	if (p.ContentContractRef == "") != (p.ContentRequirements == "") || len(p.ContentContractRef) > 512 || len(p.ContentRequirements) > 4096 || !utf8.ValidString(p.ContentContractRef+p.ContentRequirements) ||
		strings.IndexFunc(p.ContentContractRef+p.ContentRequirements, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return fmt.Errorf("runtime validation proof preparation requires bounded owner content contract and requirements together")
	}
	required := make([]string, 0, len(p.CheckAttestations))
	for _, attestation := range p.CheckAttestations {
		required = append(required, attestation.RequiredCheckRef)
	}
	if err := ValidateRuntimeValidationCheckAttestationBindings(rawAttestations, required); err != nil {
		return err
	}
	constraint := p.ProspectiveTargetSatisfactionConstraint
	if err := constraint.Validate(); err != nil {
		return fmt.Errorf("runtime validation proof preparation target constraint: %w", err)
	}
	if constraint.ValidationRecipeID != p.ValidationRecipeID {
		return fmt.Errorf("runtime validation proof preparation recipe does not match target constraint")
	}
	if p.OriginalDiagnostic != nil {
		if err := p.OriginalDiagnostic.Validate(); err != nil {
			return fmt.Errorf("runtime validation proof preparation diagnostic: %w", err)
		}
		if p.OriginalDiagnostic.OwnerModuleID != constraint.OwnerModuleID || p.OriginalDiagnostic.ObservedRevision != constraint.CatalogObservedRevision {
			return fmt.Errorf("runtime validation proof preparation diagnostic must match its owner and catalog revision")
		}
	}
	if p.TargetRef != "" {
		exactTargetDeclared := false
		for _, clause := range constraint.AllOf {
			for _, predicate := range clause.AllOf {
				if predicate.Kind == RuntimeProspectiveTargetPredicatePathExact && predicate.Value == p.TargetRef {
					exactTargetDeclared = true
				}
			}
		}
		if !exactTargetDeclared {
			return fmt.Errorf("runtime validation proof preparation target %q is absent from exact target constraint", p.TargetRef)
		}
	}
	return nil
}

func proofPreparationDiagnosticReadFixture(t testing.TB) RuntimeValidationProofPreparation {
	t.Helper()
	value := runtimeExactProofClonePreparationFixture("recipe").Normalized()
	value.OriginalDiagnostic = diagnosticFieldNormalizationFixture(16)
	value.OriginalDiagnostic.OwnerModuleID = value.ProspectiveTargetSatisfactionConstraint.OwnerModuleID
	value.OriginalDiagnostic.ObservedRevision = value.ProspectiveTargetSatisfactionConstraint.CatalogObservedRevision
	if err := referenceRuntimeValidationProofPreparationValidate(value); err != nil {
		t.Fatalf("缺少合法原诊断 preparation 前像：%v", err)
	}
	return value
}

func assertProofPreparationDiagnosticReadReference(t testing.TB, value RuntimeValidationProofPreparation) {
	t.Helper()
	before := cloneRuntimeValidationProofPreparationRaw(value)
	want := referenceRuntimeValidationProofPreparationValidate(value)
	for range 2 {
		assertProofCollectionIdentityErrors(t, value.Validate(), want)
	}
	if !reflect.DeepEqual(value, before) {
		t.Fatal("preparation 纯校验改写原诊断或嵌套 proof")
	}
	if !reflect.DeepEqual(value.Normalized().OriginalDiagnostic, value.OriginalDiagnostic) || !reflect.DeepEqual(value.Clone().OriginalDiagnostic, value.OriginalDiagnostic) {
		t.Fatal("公开 preparation Normalized／Clone 改写 raw 诊断表示")
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeValidationProofPreparation
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatal(err)
	}
	assertProofCollectionIdentityErrors(t, reloaded.Validate(), referenceRuntimeValidationProofPreparationValidate(reloaded))
}

func TestProofPreparationDiagnosticReadKeepsCompleteGateAndRawOwnerBinding(t *testing.T) {
	original := proofPreparationDiagnosticReadFixture(t)
	for _, scenario := range []string{"valid", "nil", "empty", "owner-whitespace", "revision-whitespace", "owner-drift", "revision-drift", "unsafe-field", "long-message", "invalid-utf8", "nul", "raw-output", "constraint-before-diagnostic", "attestation-before-diagnostic", "content-before-diagnostic"} {
		t.Run(scenario, func(t *testing.T) {
			value := original.Clone()
			switch scenario {
			case "nil":
				value.OriginalDiagnostic = nil
			case "empty":
				value.OriginalDiagnostic = &RuntimeOriginalDiagnostic{}
			case "owner-whitespace":
				value.OriginalDiagnostic.OwnerModuleID = " " + value.OriginalDiagnostic.OwnerModuleID + " "
			case "revision-whitespace":
				value.OriginalDiagnostic.ObservedRevision = " " + value.OriginalDiagnostic.ObservedRevision + " "
			case "owner-drift":
				value.OriginalDiagnostic.OwnerModuleID = RuntimeModuleFile
			case "revision-drift":
				value.OriginalDiagnostic.ObservedRevision = "different"
			case "unsafe-field":
				value.OriginalDiagnostic.FieldViolations[0].Actual = "authorization: 不可持久化材料"
			case "long-message":
				value.OriginalDiagnostic.OriginalMessage = strings.Repeat("原", 32768)
			case "invalid-utf8":
				value.OriginalDiagnostic.OriginalMessage = string([]byte{0xff, 0x80})
			case "nul":
				value.OriginalDiagnostic.OriginalMessage = "原\x00材料"
			case "raw-output":
				value.OriginalDiagnostic.ValidationDiagnostic = &ValidationFailureDiagnostic{StdoutPreview: "原输出"}
			case "constraint-before-diagnostic":
				value.ProspectiveTargetSatisfactionConstraint.AllOf[0].AllOf = nil
				value.OriginalDiagnostic = &RuntimeOriginalDiagnostic{}
			case "attestation-before-diagnostic":
				value.CheckAttestations[0].NativeCheckRef = ""
				value.OriginalDiagnostic = &RuntimeOriginalDiagnostic{}
			case "content-before-diagnostic":
				value.ContentContractRef = "contract"
				value.OriginalDiagnostic = &RuntimeOriginalDiagnostic{}
			}
			if scenario == "valid" || scenario == "nil" {
				if err := value.Validate(); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "owner-whitespace" || scenario == "revision-whitespace" {
				if err := value.Validate(); err == nil || err.Error() != "runtime validation proof preparation diagnostic must match its owner and catalog revision" {
					t.Fatalf("原 raw owner/revision fence 被 trim 放宽：%v", err)
				}
			}
			assertProofPreparationDiagnosticReadReference(t, value)
		})
	}
	original.OriginalDiagnostic.FieldViolations[0].Actual = "authorization: 后续材料"
	if err := original.Validate(); err == nil {
		t.Fatal("后续诊断篡改借用前次校验成功")
	}
	assertProofPreparationDiagnosticReadReference(t, original)
}

func TestProofPreparationDiagnosticReadKeepsCloneIsolationAndReducesAllocations(t *testing.T) {
	value := proofPreparationDiagnosticReadFixture(t)
	cloned := value.Clone()
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(value.OriginalDiagnostic), reflect.ValueOf(cloned.OriginalDiagnostic), "OriginalDiagnostic")
	cloned.OriginalDiagnostic.FieldViolations[0].Allowed[0] = "different"
	if reflect.DeepEqual(cloned.OriginalDiagnostic, value.OriginalDiagnostic) {
		t.Fatal("公开 raw Clone 泄漏原诊断")
	}
	original := testing.AllocsPerRun(20, func() { _ = referenceRuntimeValidationProofPreparationValidate(value) })
	current := testing.AllocsPerRun(20, func() { _ = value.Validate() })
	if current >= original {
		t.Fatalf("preparation 只读诊断分配未减少：current=%v original=%v", current, original)
	}
}

func FuzzProofPreparationDiagnosticReadReference(f *testing.F) {
	f.Add(uint8(0), " 原材料\r\n\x00 ", " b ")
	f.Add(uint8(1), " shell ", " owner-revision ")
	f.Add(uint8(2), string([]byte{0xff, 0x80}), "authorization:")
	f.Fuzz(func(t *testing.T, selector uint8, first, second string) {
		value := proofPreparationDiagnosticReadFixture(t)
		switch selector % 4 {
		case 0:
			value.OriginalDiagnostic.OriginalMessage = first
			value.OriginalDiagnostic.FieldViolations[0].Allowed = []string{first, second, first, " "}
		case 1:
			value.OriginalDiagnostic.OwnerModuleID, value.OriginalDiagnostic.ObservedRevision = first, second
		case 2:
			value.OriginalDiagnostic.FieldViolations[0].Actual = first
			value.OriginalDiagnostic.FieldViolations[0].Path = second
		case 3:
			value.ContentContractRef, value.ContentRequirements = first, second
			value.OriginalDiagnostic.Signature = second
		}
		assertProofPreparationDiagnosticReadReference(t, value)
	})
}

func BenchmarkProofPreparationDiagnosticRead(b *testing.B) {
	value := proofPreparationDiagnosticReadFixture(b)
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if mode == "reference" {
					err = referenceRuntimeValidationProofPreparationValidate(value)
				} else {
					err = value.Validate()
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

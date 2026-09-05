package workorchruntimecontract

import (
	"strings"
	"testing"
)

func TestRuntimeOperationValueRuleCatalogValidatesEveryDescriptorBackedCandidateContract(t *testing.T) {
	t.Parallel()
	checked := 0
	for _, descriptor := range RuntimeOperationDescriptors() {
		contract, found := RuntimeOperationContractForKind(descriptor.Kind)
		if !found {
			continue
		}
		checked++
		if err := validateRuntimeOperationValueRuleDeclarations(contract.Kind, contract.Parameters, contract.ValueRules, contract.Variants); err != nil {
			t.Fatalf("operation contract %q value rules: %v", contract.Kind, err)
		}
	}
	if checked == 0 {
		t.Fatal("descriptor-backed candidate operation contract catalog is empty")
	}
}

func TestRuntimeOperationValueRuleDeclarationRejectsDuplicateAndCrossTypeRelation(t *testing.T) {
	t.Parallel()
	parameters := []RuntimeOperationParameterContract{
		{Name: "left", Type: RuntimeOperationParameterTypeString},
		{Name: "right", Type: RuntimeOperationParameterTypeInteger},
	}
	distinct := runtimeDistinctValueRule("test/op", "", "left", "right")
	if err := validateRuntimeOperationValueRuleDeclarations("test/op", parameters, []RuntimeOperationValueRule{distinct}, nil); err == nil || !strings.Contains(err.Error(), "same type") {
		t.Fatalf("cross-type distinct rule error = %v", err)
	}
	parameters[1].Type = RuntimeOperationParameterTypeString
	if err := validateRuntimeOperationValueRuleDeclarations("test/op", parameters, []RuntimeOperationValueRule{distinct, distinct}, nil); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate rule error = %v", err)
	}
	hiddenBound := runtimeNonEmptyValueRule("test/op", "", "left")
	hiddenBound.Minimum = 1
	if err := validateRuntimeOperationValueRuleDeclarations("test/op", parameters, []RuntimeOperationValueRule{hiddenBound}, nil); err == nil || !strings.Contains(err.Error(), "invalid non-empty shape") {
		t.Fatalf("inactive hidden bound error = %v", err)
	}
}

func TestRuntimeOperationValueRuleViolationRejectsIncompatibleClosedShapes(t *testing.T) {
	t.Parallel()
	violation := RuntimeOperationValueRuleViolation{
		RuleRef:       "operation_value/test/op/non_empty/value",
		OperationKind: "test/op",
		ParameterName: "value",
		ActualShape:   RuntimeOperationValueActualShapeEmpty,
		RequiredShape: RuntimeOperationValueRequiredShapeIntegerRange,
	}
	if err := violation.Validate(); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("incompatible violation shape error = %v", err)
	}
}

func TestRuntimeOperationValueRuleAcceptsOnlyPossibleBoundDirection(t *testing.T) {
	t.Parallel()
	rule := runtimeIntegerRangeValueRule("test/op", "", "value", 1, 0, false)
	if !rule.AcceptsViolationShapes(RuntimeOperationValueActualShapeBelowMinimum, RuntimeOperationValueRequiredShapeIntegerRange) {
		t.Fatal("minimum-only rule rejected its possible below-minimum shape")
	}
	if rule.AcceptsViolationShapes(RuntimeOperationValueActualShapeAboveMaximum, RuntimeOperationValueRequiredShapeIntegerRange) {
		t.Fatal("minimum-only rule accepted an impossible above-maximum shape")
	}
}

func TestRuntimeOperationStringByteRuleUsesSemanticMinimumAndRawMaximum(t *testing.T) {
	t.Parallel()
	contract, found := RuntimeOperationContractForKind(StepKindWebSearch)
	if !found {
		t.Fatal("web/search operation contract is missing")
	}
	authorization := RuntimeCandidateOperationManifest{
		Kind: contract.Kind, Parameters: contract.Parameters, Variants: contract.Variants, ValueRules: contract.ValueRules,
	}.Normalized()

	blank := "   "
	operation := RuntimeOperation{Kind: StepKindWebSearch, Parameters: []OperationParameter{{Name: "query", String: &blank}}}
	violation, err := RuntimeOperationValueRuleViolationForOperation(operation, authorization)
	if err != nil || violation == nil || violation.ActualShape != RuntimeOperationValueActualShapeBelowMinimum {
		t.Fatalf("blank query violation = %+v, err=%v", violation, err)
	}

	oversizedByPadding := " " + strings.Repeat("q", 4096)
	operation.Parameters[0].String = &oversizedByPadding
	violation, err = RuntimeOperationValueRuleViolationForOperation(operation, authorization)
	if err != nil || violation == nil || violation.ActualShape != RuntimeOperationValueActualShapeAboveMaximum {
		t.Fatalf("raw-byte upper-bound violation = %+v, err=%v", violation, err)
	}
}

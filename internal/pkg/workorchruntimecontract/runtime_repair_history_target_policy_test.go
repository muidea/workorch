package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestRepairHistoryUsesOnlyOriginalPolicyTargetSet(t *testing.T) {
	for _, policy := range []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget} {
		t.Run(string(policy), func(t *testing.T) {
			binding := CoverageEvidenceBinding{TargetRefs: []string{"implementation.go"}}
			if policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
				binding.TargetRefs, binding.ProspectiveTargetRefs = nil, []string{"input_test.go"}
			}
			repair := RuntimeSemanticValidationFailureRepair{RepairTargetPolicy: policy}
			expected := append(append([]string(nil), binding.TargetRefs...), binding.ProspectiveTargetRefs...)
			targets, err := runtimeRepairHistoryBindingTargets(binding, repair)
			if err != nil || !reflect.DeepEqual(targets, expected) {
				t.Fatalf("exact original target set: %v %v", targets, err)
			}
			targets[0] = "changed"
			if reflect.DeepEqual(targets, append(append([]string(nil), binding.TargetRefs...), binding.ProspectiveTargetRefs...)) {
				t.Fatal("target resolver aliases original binding")
			}
			for _, bad := range []CoverageEvidenceBinding{
				{}, {TargetRefs: []string{"implementation.go"}, ProspectiveTargetRefs: []string{"input_test.go"}},
				{TargetRefs: []string{"input_test.go"}}, {ProspectiveTargetRefs: []string{"implementation.go"}},
			} {
				// A same-field different target is not rejected by this shape
				// helper; the enclosing consumer compares it with the exact
				// closure, owner confirmation and all-of receipts independently.
				if policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget && len(bad.TargetRefs) != 0 && len(bad.ProspectiveTargetRefs) == 0 || policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget && len(bad.TargetRefs) == 0 && len(bad.ProspectiveTargetRefs) != 0 {
					continue
				}
				if _, err := runtimeRepairHistoryBindingTargets(bad, repair); err == nil {
					t.Fatal("empty, mixed or wrong-policy targets accepted")
				}
			}
		})
	}
	for _, binding := range []CoverageEvidenceBinding{{TargetRefs: []string{" b "}}, {TargetRefs: []string{"b", "a"}}, {TargetRefs: []string{"a", "a"}}} {
		if _, err := runtimeRepairHistoryBindingTargets(binding, RuntimeSemanticValidationFailureRepair{RepairTargetPolicy: RuntimeOperationTargetPolicyExistingOwnerScope}); err == nil {
			t.Fatal("noncanonical target set accepted")
		}
	}
	if _, err := runtimeRepairHistoryBindingTargets(CoverageEvidenceBinding{TargetRefs: []string{"a"}}, RuntimeSemanticValidationFailureRepair{}); err == nil {
		t.Fatal("missing sealed policy guessed from nonempty targets")
	}
}

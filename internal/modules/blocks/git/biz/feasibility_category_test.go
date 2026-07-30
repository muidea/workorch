package git

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerFeasibilityCategoriesAreTotal(t *testing.T) {
	wantCodes := []string{"git_repository_unavailable", "operation_admissibility_binding_unavailable", "operation_admissibility_stale", "git_operation_contract_missing", "git_operation_variant_invalid", "git_operation_variant_not_admissible"}
	if len(ownerFeasibilityCategories) != len(wantCodes) {
		t.Fatalf("owner feasibility registry size=%d want=%d", len(ownerFeasibilityCategories), len(wantCodes))
	}
	for _, code := range wantCodes {
		category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code})
		if !runtimecontract.ValidRuntimeOwnerFeasibilityCategory(category) || category == runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("declared code %q category=%q", code, category)
		}
	}
	for _, code := range []string{"", "operation_target_missing", "owner_new_unmapped_code"} {
		if category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code}); category != runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("undeclared or cross-owner code %q category=%q, want owner contract defect", code, category)
		}
	}
}

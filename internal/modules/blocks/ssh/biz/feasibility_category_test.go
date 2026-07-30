package ssh

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerFeasibilityCategoriesAreTotal(t *testing.T) {
	wantCodes := []string{"ssh_prerequisite_unavailable", "operation_admissibility_stale", "ssh_operation_invalid"}
	if len(ownerFeasibilityCategories) != len(wantCodes) {
		t.Fatalf("owner feasibility registry size=%d want=%d", len(ownerFeasibilityCategories), len(wantCodes))
	}
	for _, code := range wantCodes {
		category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code})
		if !runtimecontract.ValidRuntimeOwnerFeasibilityCategory(category) || category == runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("declared code %q category=%q", code, category)
		}
	}
	for _, code := range []string{"", "web_operation_invalid", "owner_new_unmapped_code"} {
		if category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code}); category != runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("undeclared or cross-owner code %q category=%q, want owner contract defect", code, category)
		}
	}
}

package shell

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerFeasibilityCategoriesAreTotal(t *testing.T) {
	wantCodes := []string{"validation_recipe_unavailable", "validation_target_missing", "shell_working_directory_unavailable", "shell_executable_unavailable", "operation_admissibility_stale", "shell_operation_invalid", "shell_command_invalid", "shell_working_directory_invalid", "shell_command_policy_rejected", "shell_validation_contract_rejected", "shell_command_timeout_policy_rejected"}
	if len(ownerFeasibilityCategories) != len(wantCodes) {
		t.Fatalf("owner feasibility registry size=%d want=%d", len(ownerFeasibilityCategories), len(wantCodes))
	}
	for _, code := range wantCodes {
		category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code})
		if !runtimecontract.ValidRuntimeOwnerFeasibilityCategory(category) || category == runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("declared code %q category=%q", code, category)
		}
	}
	for _, code := range []string{"", "git_repository_unavailable", "owner_new_unmapped_code"} {
		if category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code}); category != runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("undeclared or cross-owner code %q category=%q, want owner contract defect", code, category)
		}
	}
}

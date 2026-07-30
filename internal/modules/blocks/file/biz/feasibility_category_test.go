package file

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerFeasibilityCategoriesAreTotal(t *testing.T) {
	wantCodes := []string{
		"operation_target_missing", "patch_target_missing", "operation_target_not_regular", "patch_parent_conflict", "patch_binary_target",
		"operation_admissibility_unavailable", "operation_target_unreadable", "operation_base_hash_unavailable", "patch_read_failed", "patch_write_failed",
		"operation_admissibility_stale", "operation_replay_proof_invalid", "operation_replay_proof_unavailable", "operation_base_hash_mismatch",
		"patch_basehash_mismatch", "patch_text_replace_conflict", "patch_unified_diff_conflict", "unified_diff_apply_conflict", "unified_diff_infeasible",
		"operation_target_invalid", "operation_target_not_admissible", "patch_invalid_path", "patch_type_unsupported", "patch_target_exists",
		"patch_invalid_input", "unified_diff_invalid", "unified_diff_target_mismatch",
	}
	assertOwnerFeasibilityCategories(t, wantCodes)
}

func assertOwnerFeasibilityCategories(t *testing.T, wantCodes []string) {
	t.Helper()
	if len(ownerFeasibilityCategories) != len(wantCodes) {
		t.Fatalf("owner feasibility registry size=%d want=%d", len(ownerFeasibilityCategories), len(wantCodes))
	}
	for _, code := range wantCodes {
		category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code})
		if !runtimecontract.ValidRuntimeOwnerFeasibilityCategory(category) || category == runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("declared code %q category=%q", code, category)
		}
	}
	for _, code := range []string{"", "shell_operation_invalid", "owner_new_unmapped_code"} {
		if category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code}); category != runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("undeclared or cross-owner code %q category=%q, want owner contract defect", code, category)
		}
	}
}

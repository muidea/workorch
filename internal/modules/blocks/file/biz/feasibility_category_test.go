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
		"operation_base_hash_required", "patch_basehash_mismatch", "patch_text_replace_conflict", "patch_unified_diff_conflict", "unified_diff_apply_conflict", "unified_diff_infeasible",
		"operation_target_invalid", "operation_target_not_admissible", "patch_invalid_path", "patch_type_unsupported", "patch_target_exists",
		"operation_target_exists", "patch_invalid_input",
	}
	assertOwnerFeasibilityCategories(t, wantCodes)
	for _, code := range []string{"unified_diff_invalid", "unified_diff_target_mismatch"} {
		if category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code}); category != runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("conformance-fenced code %q category=%q, want owner contract defect", code, category)
		}
	}
}

func assertOwnerFeasibilityCategories(t *testing.T, wantCodes []string) {
	t.Helper()
	if len(ownerFeasibilityCategories) != len(wantCodes)+2 {
		t.Fatalf("owner feasibility registry size=%d want=%d", len(ownerFeasibilityCategories), len(wantCodes)+2)
	}
	for _, code := range wantCodes {
		failure := &runtimecontract.FailureFact{Code: code}
		category := ownerFeasibilityCategory(failure)
		if !runtimecontract.ValidRuntimeOwnerFeasibilityCategory(category) || category == runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("declared code %q category=%q", code, category)
		}
		impact := ownerFeasibilityConstraintImpact(failure)
		if code == "operation_target_exists" && impact != runtimecontract.RuntimeOwnerFeasibilityConstraintImpactSealedTargetAuthorizationContradicted {
			t.Fatalf("existing-target code impact=%q", impact)
		}
		if code != "operation_target_exists" && impact != "" {
			t.Fatalf("candidate-local code %q unexpectedly contradicted sealed target authorization: %q", code, impact)
		}
	}
	for _, code := range []string{"", "shell_operation_invalid", "owner_new_unmapped_code"} {
		if category := ownerFeasibilityCategory(&runtimecontract.FailureFact{Code: code}); category != runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
			t.Fatalf("undeclared or cross-owner code %q category=%q, want owner contract defect", code, category)
		}
	}
	if impact := ownerFeasibilityConstraintImpact(nil); impact != "" {
		t.Fatalf("nil failure impact=%q", impact)
	}
}

package file

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ownerFeasibilityCategory is total at the owner boundary. An undeclared
// emitted code is not downgraded to a generic EventHub error: it crosses the
// concrete feasibility Result as an owner-contract-defect with the original
// code/message retained in RuntimeFeedback.
func ownerFeasibilityCategory(failure *runtimecontract.FailureFact) runtimecontract.RuntimeOwnerFeasibilityCategory {
	if failure == nil {
		return ""
	}
	code := strings.TrimSpace(failure.Code)
	category, found := ownerFeasibilityCategories[code]
	if !found {
		return runtimecontract.RuntimeOwnerFeasibilityCategoryOwnerContractDefect
	}
	return category
}

// ownerFeasibilityConstraintImpact maps only File-owned factual codes to the
// sealed owner-neutral constraint they disprove.  The Orchestrator consumes
// this typed fact; it never parses File codes or messages to choose an SM-2
// transition.
func ownerFeasibilityConstraintImpact(failure *runtimecontract.FailureFact) runtimecontract.RuntimeOwnerFeasibilityConstraintImpact {
	if failure == nil {
		return ""
	}
	switch strings.TrimSpace(failure.Code) {
	case "operation_target_exists":
		return runtimecontract.RuntimeOwnerFeasibilityConstraintImpactSealedTargetAuthorizationContradicted
	default:
		return ""
	}
}

var ownerFeasibilityCategories = map[string]runtimecontract.RuntimeOwnerFeasibilityCategory{
	"operation_target_missing":            runtimecontract.RuntimeOwnerFeasibilityCategoryResourceAbsent,
	"patch_target_missing":                runtimecontract.RuntimeOwnerFeasibilityCategoryResourceAbsent,
	"operation_target_not_regular":        runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular,
	"patch_parent_conflict":               runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular,
	"patch_binary_target":                 runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular,
	"operation_admissibility_unavailable": runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"operation_target_unreadable":         runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"operation_base_hash_unavailable":     runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"patch_read_failed":                   runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"patch_write_failed":                  runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"operation_admissibility_stale":       runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"operation_replay_proof_invalid":      runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"operation_replay_proof_unavailable":  runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"operation_base_hash_mismatch":        runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"operation_base_hash_required":        runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"operation_target_exists":             runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"patch_basehash_mismatch":             runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"patch_text_replace_conflict":         runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"patch_unified_diff_conflict":         runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"unified_diff_apply_conflict":         runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"unified_diff_infeasible":             runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"operation_target_invalid":            runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"operation_target_not_admissible":     runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"patch_invalid_path":                  runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"patch_type_unsupported":              runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"patch_target_exists":                 runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"patch_invalid_input":                 runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"unified_diff_invalid":                runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"unified_diff_target_mismatch":        runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
}

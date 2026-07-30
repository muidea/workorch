package shell

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ownerFeasibilityCategory is total at the owner boundary. An undeclared
// emitted code crosses the concrete feasibility Result as an
// owner-contract-defect, retaining its original diagnostic.
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

var ownerFeasibilityCategories = map[string]runtimecontract.RuntimeOwnerFeasibilityCategory{
	"validation_recipe_unavailable":         runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable,
	"validation_target_missing":             runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable,
	"shell_working_directory_unavailable":   runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"shell_executable_unavailable":          runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"operation_admissibility_stale":         runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"shell_operation_invalid":               runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"shell_command_invalid":                 runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"shell_working_directory_invalid":       runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"shell_command_policy_rejected":         runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"shell_validation_contract_rejected":    runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"shell_command_timeout_policy_rejected": runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
}

package git

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
	"git_repository_unavailable":                  runtimecontract.RuntimeOwnerFeasibilityCategoryResourceAbsent,
	"git_workspace_tree_unavailable":              runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"operation_admissibility_binding_unavailable": runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"operation_admissibility_stale":               runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"git_operation_contract_missing":              runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"git_operation_variant_invalid":               runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"git_operation_variant_not_admissible":        runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
}

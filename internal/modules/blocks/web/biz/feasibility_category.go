package web

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
	"web_provider_configuration_unavailable": runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"web_operation_infeasible":               runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"operation_admissibility_stale":          runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"web_operation_invalid":                  runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
}

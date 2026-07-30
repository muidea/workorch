package mcp

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
	"mcp_server_catalog_unavailable": runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"mcp_prerequisite_unavailable":   runtimecontract.RuntimeOwnerFeasibilityCategoryEnvironmentUnavailable,
	"operation_admissibility_stale":  runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown,
	"mcp_operation_invalid":          runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
	"mcp_server_not_admissible":      runtimecontract.RuntimeOwnerFeasibilityCategoryPolicyDisallowed,
}

package activationports

import (
	"fmt"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const executionLimitsAdmissionRef = "execution-limits:effective"

func runtimeExecutionLimitsAdmissionBinding(limits runtimecontract.RuntimeExecutionLimits) workspacemodel.DefinitionAdmissionPolicyBinding {
	return workspacemodel.DefinitionAdmissionPolicyBinding{
		PolicyRef: executionLimitsAdmissionRef, Revision: limits.ProtocolRef,
		SnapshotHash: runtimecontract.StableRuntimeExecutionLimitsHash(limits),
	}
}

func validateRuntimeExecutionLimitsStartupSeal(candidate workspacemodel.DefinitionAdmissionCandidate, limits *runtimecontract.RuntimeExecutionLimits) error {
	count := 0
	for _, binding := range candidate.AdmissionPolicyBindings {
		if binding.PolicyRef != executionLimitsAdmissionRef {
			continue
		}
		count++
		if limits == nil || limits.Validate() != nil || binding != runtimeExecutionLimitsAdmissionBinding(*limits) {
			return fmt.Errorf("%w: execution limits differ from Workspace admission", common.ErrConflict)
		}
	}
	if (limits == nil && count != 0) || (limits != nil && count != 1) {
		return fmt.Errorf("%w: execution limits admission binding is missing or duplicated", common.ErrConflict)
	}
	return nil
}

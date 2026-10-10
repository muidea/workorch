package activationports

import (
	"fmt"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const executionLimitsAdmissionRef = "execution-limits:effective"

func runtimeExecutionLimits(input map[string]any) *runtimecontract.RuntimeExecutionLimits {
	raw, present := input["executionLimits"]
	if !present {
		return nil
	}
	value := runtimeMap(raw)
	return &runtimecontract.RuntimeExecutionLimits{
		ProtocolRef:                runtimeString(value["protocolRef"]),
		MaxProviderCalls:           runtimeInt(value["maxProviderCalls"]),
		MaxCapabilityOperations:    runtimeInt(value["maxCapabilityOperations"]),
		MaxProofBatchesPerCWI:      runtimeInt(value["maxProofBatchesPerCWI"]),
		MaxDurableDispatchesPerCWI: runtimeInt(value["maxDurableDispatchesPerCWI"]),
	}
}

// Shape and range checks run on both raw sources before template precedence
// can hide an invalid declaration, and independently at final activation.
func validateRuntimeExecutionLimitsInput(input map[string]any) error {
	raw, present := input["executionLimits"]
	if !present {
		return nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: executionLimits requires an object", common.ErrInvalidArgument)
	}
	for key := range value {
		switch key {
		case "protocolRef", "maxProviderCalls", "maxCapabilityOperations", "maxProofBatchesPerCWI", "maxDurableDispatchesPerCWI":
		default:
			return fmt.Errorf("%w: unknown executionLimits field %q", common.ErrInvalidArgument, key)
		}
	}
	for _, key := range []string{"maxProofBatchesPerCWI", "maxDurableDispatchesPerCWI"} {
		if bound, present := value[key]; present {
			if !runtimeIntegerHasExactShape(bound) || runtimeInt(bound) < 1 {
				return fmt.Errorf("%w: executionLimits.%s must be a positive exact integer", common.ErrInvalidArgument, key)
			}
		}
	}
	protocol, ok := value["protocolRef"].(string)
	if !ok || protocol != runtimecontract.RuntimeExecutionLimitsV1 {
		return fmt.Errorf("%w: executionLimits.protocolRef must select the current protocol", common.ErrInvalidArgument)
	}
	for _, key := range []string{"maxProviderCalls", "maxCapabilityOperations"} {
		if !runtimeIntegerHasExactShape(value[key]) {
			return fmt.Errorf("%w: executionLimits.%s must be an exact integer", common.ErrInvalidArgument, key)
		}
	}
	if err := runtimeExecutionLimits(input).Validate(); err != nil {
		return fmt.Errorf("%w: executionLimits: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

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

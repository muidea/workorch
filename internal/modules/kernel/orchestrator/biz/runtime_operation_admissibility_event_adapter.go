package orchestrator

// Operation admissibility adapter is internal to the owner Biz.

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func AcquireRuntimeOperationAdmissibilityPrerequisite(ctx context.Context, hub magicevent.Hub, command orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand) (orchestratorevents.AcquireRuntimeOperationAdmissibilityResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.AuthoritySnapshotID) == "" ||
		strings.TrimSpace(command.PrerequisiteID) == "" || strings.TrimSpace(command.OwnerModuleID) == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" || len(command.RequiredKinds) == 0 || command.MaxItems < 1 || command.MaxItems > runtimecontract.RuntimeCandidateContextItemsMaxCount {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, common.ErrInvalidArgument
	}
	if err := validateAdmissibilityOwnerKinds(command.OwnerModuleID, command.RequiredKinds); err != nil {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility,
		ModuleID,
		command.OwnerModuleID,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.PrerequisiteID != command.PrerequisiteID {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, fmt.Errorf("runtime operation admissibility prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	inventory := response.Admissibility.Normalized()
	if inventory.PrerequisiteID != command.PrerequisiteID || inventory.OwnerModuleID != command.OwnerModuleID {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, fmt.Errorf("runtime operation admissibility prerequisite %q returned a mismatched owner inventory", command.PrerequisiteID)
	}
	if err := inventory.Validate(); err != nil {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, err
	}
	if binding := strings.TrimSpace(command.CapabilityStateBinding); binding != "" && inventory.AcquisitionBinding != binding {
		return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, fmt.Errorf("runtime operation admissibility prerequisite %q returned a mismatched capability-state binding", command.PrerequisiteID)
	}
	for _, required := range command.RequiredKinds {
		if !containsRuntimeOperationKind(inventory.OperationKinds, required) {
			return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, fmt.Errorf("runtime operation admissibility prerequisite %q omitted kind %q", command.PrerequisiteID, required)
		}
	}
	response.Admissibility = inventory
	if response.Observation != nil {
		observation := response.Observation.Normalized()
		if observation.PrerequisiteID != inventory.PrerequisiteID || observation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility || observation.OwnerModuleID != inventory.OwnerModuleID {
			return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, fmt.Errorf("runtime operation admissibility prerequisite %q returned a mismatched freshness observation", command.PrerequisiteID)
		}
		if err := observation.Validate(); err != nil {
			return orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{}, err
		}
		response.Observation = &observation
	}
	return response, nil
}

func ProbeRuntimeOperationFeasibility(ctx context.Context, hub magicevent.Hub, command orchestratorevents.ProbeRuntimeOperationFeasibilityCommand) (orchestratorevents.ProbeRuntimeOperationFeasibilityResult, error) {
	operation := command.Operation.Normalized()
	inventory := command.Admissibility.Normalized()
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" ||
		strings.TrimSpace(command.Transaction.AuthoritySnapshotID) == "" || operation.ID == "" || operation.Kind == "" {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, common.ErrInvalidArgument
	}
	descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !ok || inventory.OwnerModuleID == "" || inventory.OwnerModuleID != descriptor.OwnerModuleID || !containsRuntimeOperationKind(inventory.OperationKinds, operation.Kind) {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility probe has no matching owner admissibility inventory")
	}
	if err := inventory.Validate(); err != nil {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	command.Operation = operation
	command.Admissibility = inventory
	if command.Observation != nil {
		observation := command.Observation.Normalized()
		if observation.PrerequisiteID != inventory.PrerequisiteID || observation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility || observation.OwnerModuleID != inventory.OwnerModuleID {
			return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility probe has mismatched freshness observation")
		}
		if err := observation.Validate(); err != nil {
			return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, err
		}
		command.Observation = &observation
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility,
		ModuleID,
		descriptor.OwnerModuleID,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.ProbeRuntimeOperationFeasibilityResult)
	if !ok || response.OperationID != operation.ID || response.AdmissibilitySnapshotID != inventory.SnapshotID || response.CapabilityStateBinding != inventory.AcquisitionBinding {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility probe returned an invalid result")
	}
	if response.Feasible {
		if response.Failure != nil || response.Stale != nil {
			return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("feasible runtime operation probe returned failure facts")
		}
		if response.ReplayProof != nil {
			proof := response.ReplayProof.Normalized()
			if err := proof.Validate(); err != nil {
				return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, err
			}
			if proof.OperationID != operation.ID || proof.AdmissibilitySnapshotID != inventory.SnapshotID || proof.AcquisitionBinding != inventory.AcquisitionBinding || proof.OwnerModuleID != descriptor.OwnerModuleID {
				return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility probe returned mismatched replay proof")
			}
			response.ReplayProof = &proof
		}
		return response, nil
	}
	if response.ReplayProof != nil {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("infeasible runtime operation probe cannot return replay proof")
	}
	if response.Stale != nil {
		if response.Failure != nil {
			return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("stale runtime operation probe returned ordinary failure facts")
		}
		stale := response.Stale.Normalized()
		if err := stale.Validate(); err != nil {
			return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, err
		}
		if stale.OwnerModuleID != inventory.OwnerModuleID {
			return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("stale runtime operation probe returned mismatched owner")
		}
		response.Stale = &stale
		return response, nil
	}
	if response.Failure == nil || strings.TrimSpace(string(response.Failure.Class)) == "" || strings.TrimSpace(response.Failure.Code) == "" || strings.TrimSpace(response.Failure.Signature) == "" || strings.TrimSpace(response.Failure.Message) == "" {
		return orchestratorevents.ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("infeasible runtime operation probe requires stable failure facts")
	}
	return response, nil
}

func validateAdmissibilityOwnerKinds(owner string, kinds []string) error {
	for _, kind := range kinds {
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
		if !ok || descriptor.OwnerModuleID != owner {
			return fmt.Errorf("runtime operation kind %q does not belong to admissibility owner %q", kind, owner)
		}
	}
	return nil
}

func containsRuntimeOperationKind(values []string, wanted string) bool {
	wanted = runtimecontract.CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if runtimecontract.CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}

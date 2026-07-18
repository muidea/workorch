package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func NewRuntimeOperationAdmissibility(prerequisiteID, ownerModuleID string, operationKinds, targetRefs, selectorRefs, constraintRefs []string, observedRevision string) (runtimecontract.RuntimeOperationAdmissibility, error) {
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: prerequisiteID, OwnerModuleID: ownerModuleID,
		OperationKinds: operationKinds, TargetRefs: targetRefs, SelectorRefs: selectorRefs,
		ConstraintRefs: constraintRefs, ObservedRevision: observedRevision,
	}.Normalized()
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		return runtimecontract.RuntimeOperationAdmissibility{}, err
	}
	return inventory, nil
}

func ValidateRuntimeOperationAdmissibilityCommand(command AcquireRuntimeOperationAdmissibilityCommand, ownerModuleID string) error {
	if command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.AuthoritySnapshotID) == "" ||
		strings.TrimSpace(command.PrerequisiteID) == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" ||
		strings.TrimSpace(command.OwnerModuleID) == "" || command.OwnerModuleID != ownerModuleID || len(command.RequiredKinds) == 0 || command.MaxItems < 1 || command.MaxItems > runtimecontract.RuntimeCandidateContextItemsMaxCount {
		return common.ErrInvalidArgument
	}
	for _, kind := range command.RequiredKinds {
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
		if !ok || descriptor.OwnerModuleID != ownerModuleID {
			return fmt.Errorf("runtime operation kind %q does not belong to owner %q", kind, ownerModuleID)
		}
	}
	return nil
}

func ValidateRuntimeOperationFeasibilityCommand(command ProbeRuntimeOperationFeasibilityCommand, ownerModuleID string) error {
	operation := command.Operation.Normalized()
	inventory := command.Admissibility.Normalized()
	if command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.Transaction.AuthoritySnapshotID) == "" ||
		strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" || operation.ID == "" || operation.Kind == "" || inventory.OwnerModuleID != ownerModuleID {
		return common.ErrInvalidArgument
	}
	descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !ok || descriptor.OwnerModuleID != ownerModuleID {
		return fmt.Errorf("runtime operation kind %q does not belong to owner %q", operation.Kind, ownerModuleID)
	}
	if err := inventory.Validate(); err != nil {
		return err
	}
	for _, kind := range inventory.OperationKinds {
		if kind == operation.Kind {
			return nil
		}
	}
	return fmt.Errorf("runtime operation kind %q is absent from owner admissibility inventory", operation.Kind)
}

func RuntimeOperationInfeasibleFailure(operation runtimecontract.RuntimeOperation, code, message string) *runtimecontract.FailureFact {
	operation = operation.Normalized()
	code = strings.TrimSpace(code)
	if code == "" {
		code = "operation_infeasible"
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "runtime operation is not currently feasible"
	}
	return &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateContractFailure,
		Code:  code, Signature: "operation-admissibility/" + operation.Kind + "/" + code,
		Message: message, TargetRefs: append([]string(nil), operation.TargetRefs...),
	}
}

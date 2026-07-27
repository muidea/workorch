package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func NewRuntimeOperationAdmissibility(prerequisiteID, ownerModuleID string, operationKinds, targetRefs, selectorRefs, constraintRefs []string, observedRevision string) (runtimecontract.RuntimeOperationAdmissibility, error) {
	return NewRuntimeOperationAdmissibilityWithBinding(prerequisiteID, ownerModuleID, operationKinds, targetRefs, selectorRefs, constraintRefs, observedRevision, "")
}

// NewRuntimeOperationAdmissibilityWithBinding creates an inventory attached to
// one opaque owner capability acquisition. The binding is intentionally not
// interpreted by the generic event contract.
func NewRuntimeOperationAdmissibilityWithBinding(prerequisiteID, ownerModuleID string, operationKinds, targetRefs, selectorRefs, constraintRefs []string, observedRevision, acquisitionBinding string) (runtimecontract.RuntimeOperationAdmissibility, error) {
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: prerequisiteID, OwnerModuleID: ownerModuleID,
		OperationKinds: operationKinds, TargetRefs: targetRefs, SelectorRefs: selectorRefs,
		ConstraintRefs: constraintRefs, ObservedRevision: observedRevision, AcquisitionBinding: acquisitionBinding,
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
		strings.TrimSpace(command.OwnerModuleID) == "" || command.OwnerModuleID != ownerModuleID || len(command.RequiredKinds) == 0 || command.MaxItems < 1 || command.MaxItems > runtimecontract.RuntimeOwnerInventoryItemsMaxCount {
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

// RuntimeOperationAuthorizationIdentity builds the minimal sealed authorization
// identity an owner receiver needs for kind/owner/revision checks. Production
// probes must still pass the full manifest authorization from the sealed
// candidate contract; this helper is for tests and identity-only fixtures.
func RuntimeOperationAuthorizationIdentity(kind string) (runtimecontract.RuntimeCandidateOperationManifest, error) {
	descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return runtimecontract.RuntimeCandidateOperationManifest{}, fmt.Errorf("unknown runtime operation kind %q", kind)
	}
	return runtimecontract.RuntimeCandidateOperationManifest{
		Kind: descriptor.Kind, OwnerModuleID: descriptor.OwnerModuleID,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
	}.Normalized(), nil
}

func ValidateRuntimeOperationFeasibilityCommand(command ProbeRuntimeOperationFeasibilityCommand, ownerModuleID string) error {
	operation := command.Operation.Normalized()
	inventory := command.Admissibility.Normalized()
	authorization := command.OperationAuthorization.Normalized()
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
	if strings.TrimSpace(command.CandidateManifestHash) == "" {
		return fmt.Errorf("runtime operation feasibility requires sealed candidate manifest hash")
	}
	if authorization.Kind == "" {
		return fmt.Errorf("runtime operation feasibility requires sealed operation authorization")
	}
	if runtimecontract.CanonicalRuntimeBlockKind(authorization.Kind) != operation.Kind {
		return fmt.Errorf("runtime operation feasibility authorization kind %q does not match operation %q", authorization.Kind, operation.Kind)
	}
	if authorization.OwnerModuleID != "" && authorization.OwnerModuleID != ownerModuleID {
		return fmt.Errorf("runtime operation feasibility authorization owner %q does not match receiver %q", authorization.OwnerModuleID, ownerModuleID)
	}
	if authorization.OperationContractRevision != "" {
		if err := runtimecontract.ValidateRuntimeOperationContractRevision(authorization.OperationContractRevision); err != nil {
			return err
		}
	}
	// CapabilityStateBinding vs AcquisitionBinding is an owner-local freshness
	// concern: a mismatched binding must surface as stale/infeasible observation,
	// not as a shared identity validation error. Owners still re-check it in
	// their private probe path.
	if len(authorization.AdmissibilitySnapshotRefs) != 0 {
		found := false
		for _, ref := range authorization.AdmissibilitySnapshotRefs {
			if ref == inventory.SnapshotID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("runtime operation feasibility admissibility snapshot is absent from sealed authorization")
		}
	}
	for _, kind := range inventory.OperationKinds {
		if kind == operation.Kind {
			return nil
		}
	}
	return fmt.Errorf("runtime operation kind %q is absent from owner admissibility inventory", operation.Kind)
}

// NormalizeProbeRuntimeOperationFeasibilityResult enforces the unified outer
// observation contract: feasible XOR stale XOR infeasible. Owners must not
// return mixed outcomes or choose correction/action/terminal.
func NormalizeProbeRuntimeOperationFeasibilityResult(result ProbeRuntimeOperationFeasibilityResult) (ProbeRuntimeOperationFeasibilityResult, error) {
	result.OperationID = strings.TrimSpace(result.OperationID)
	result.AdmissibilitySnapshotID = strings.TrimSpace(result.AdmissibilitySnapshotID)
	result.CapabilityStateBinding = strings.TrimSpace(result.CapabilityStateBinding)
	result.CandidateManifestHash = strings.TrimSpace(result.CandidateManifestHash)
	if result.OperationID == "" || result.AdmissibilitySnapshotID == "" {
		return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility result requires operation and admissibility identity")
	}
	active := 0
	if result.Feasible {
		active++
	}
	if result.Stale != nil {
		active++
	}
	if result.Failure != nil {
		active++
	}
	if result.PrerequisiteRequest != nil && result.Feasible {
		return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility cannot be feasible with a prerequisite request")
	}
	if result.Feasible {
		if result.Stale != nil || result.Failure != nil {
			return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility feasible result cannot carry stale or failure")
		}
		return result, nil
	}
	if result.Stale != nil {
		if result.Failure != nil {
			return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility stale result cannot also carry failure")
		}
		if err := result.Stale.Validate(); err != nil {
			return ProbeRuntimeOperationFeasibilityResult{}, err
		}
		return result, nil
	}
	if result.Failure != nil || result.PrerequisiteRequest != nil {
		if result.Failure != nil {
			result.Failure.Class = runtimecontract.FailureClass(strings.TrimSpace(string(result.Failure.Class)))
			result.Failure.Code = strings.TrimSpace(result.Failure.Code)
			result.Failure.Signature = strings.TrimSpace(result.Failure.Signature)
			result.Failure.Message = strings.TrimSpace(result.Failure.Message)
			if result.Failure.Class == "" || result.Failure.Code == "" {
				return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility infeasible result requires failure class and code")
			}
		}
		return result, nil
	}
	if active == 0 {
		return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility result must be feasible, stale, or infeasible")
	}
	return result, nil
}

// NewProbeRuntimeOperationFeasibilityResult builds the unified owner outer
// observation. Callers pass exactly one of feasible/stale/infeasible inputs.
func NewProbeRuntimeOperationFeasibilityResult(command ProbeRuntimeOperationFeasibilityCommand, feasible bool, failure *runtimecontract.FailureFact, stale *runtimecontract.RuntimePrerequisiteStale, prerequisiteRequest *runtimecontract.RuntimeOperationPrerequisiteRequest, replayProof *runtimecontract.RuntimeOperationReplayProof) (ProbeRuntimeOperationFeasibilityResult, error) {
	operation := command.Operation.Normalized()
	if feasible && (failure != nil || stale != nil || prerequisiteRequest != nil) {
		return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility cannot mix feasible with failure, stale, or prerequisite request")
	}
	if stale != nil && (failure != nil || prerequisiteRequest != nil) {
		return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility cannot mix stale with failure or prerequisite request")
	}
	if !feasible && failure == nil && stale == nil && prerequisiteRequest == nil {
		return ProbeRuntimeOperationFeasibilityResult{}, fmt.Errorf("runtime operation feasibility result must be feasible, stale, or infeasible")
	}
	// Capability identity on the outer observation is the inventory acquisition
	// binding the owner validated against. A mismatched command binding is an
	// owner-local stale/infeasible signal and must not rewrite this identity.
	result := ProbeRuntimeOperationFeasibilityResult{
		OperationID:             operation.ID,
		AdmissibilitySnapshotID: command.Admissibility.SnapshotID,
		CapabilityStateBinding:  command.Admissibility.AcquisitionBinding,
		CandidateManifestHash:   command.CandidateManifestHash,
		Feasible:                feasible,
		ReplayProof:             replayProof,
		Failure:                 failure,
		Stale:                   stale,
		PrerequisiteRequest:     prerequisiteRequest,
	}
	if result.Feasible {
		result.Failure = nil
		result.Stale = nil
		result.PrerequisiteRequest = nil
	}
	return NormalizeProbeRuntimeOperationFeasibilityResult(result)
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

package runtime

import (
	"fmt"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CompileRuntimeValidationInputSnapshotSource reverse-proves the actual
// evidence-committed validation operation and its current exact binding, then
// derives the original input bytes from the accepted semantic batch. It does
// not read owners, persist currentness, grant credit or select a Runtime action.
func CompileRuntimeValidationInputSnapshotSource(aggregate RuntimeAggregate, dispatchID, operationID, receiptHash string) (runtimecontract.RuntimeValidationProofRequirementBinding, error) {
	source, err := CompileRuntimeOperationReadSource(aggregate, dispatchID, operationID, receiptHash)
	if err != nil {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, err
	}
	dispatch, _, found := operationReceiptDispatch(aggregate, dispatchID)
	execution, executed := operationExecutionRecord(dispatch.OperationExecutions, operationID)
	if !found || !executed || dispatch.State != DispatchStateCommitted || dispatch.ImplementationWorkCommitment != nil ||
		source.Context.EvidenceKind != runtimecontract.EvidenceKindValidation || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || len(source.Operation.BindingRefs) != 1 {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("%w: input snapshot requires original committed ordinary validation", common.ErrConflict)
	}
	binding, found := runtimecontract.CoverageEvidenceBindingForRef(dispatch.CoverageBindings, source.Operation.BindingRefs[0])
	if !found {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, common.ErrConflict
	}
	current := 0
	for _, coverage := range aggregate.Facts.Coverage {
		ref := coverage.CurrentBindingRef
		if coverage.ID == binding.CoverageWorkItemID && ref != nil && ref.BindingID == binding.ID && ref.BindingHash == binding.Hash && ref.FocusID == binding.FocusID {
			current++
		}
	}
	facts := 0
	for _, fact := range aggregate.Facts.Evidence.Accepted {
		if fact.ID == execution.EvidenceFactID && runtimecontract.StableRuntimeEvidenceFactHash(fact) == execution.EvidenceFactHash &&
			fact.Kind == runtimecontract.EvidenceKindValidation && fact.DispatchID == dispatchID && fact.OperationID == operationID &&
			slices.Contains(fact.BindingRefs, binding.ID) && slices.Contains(fact.CoverageWorkItemIDs, binding.CoverageWorkItemID) {
			facts++
		}
	}
	if current != 1 || facts != 1 {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("%w: input snapshot requires unique accepted current validation proof", common.ErrConflict)
	}
	input, err := runtimecontract.CompileRuntimeBindingValidationInputReview(binding, aggregate.SemanticPlanning)
	if err != nil {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, err
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(source.Operation.Kind)
	if !found {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, common.ErrConflict
	}
	// The review is not enough: every file must also have been read with those
	// exact bytes by the original validation owner. These are owner-neutral
	// resource/byte identities; no path, language or native-check interpretation.
	if err := validateRuntimeReviewedInputReads(input, source.OriginalDependencies, descriptor.OwnerModuleID+"/"+string(aggregate.Facts.WorkspaceID)); err != nil {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, err
	}
	return input, nil
}

func validateRuntimeReviewedInputReads(input runtimecontract.RuntimeValidationProofRequirementBinding, dependencies runtimecontract.RuntimeOwnerRevisionSet, ownerInstance string) error {
	if input.InputReview == nil || input.DefinitionRevision != dependencies.DefinitionRevision || input.Validate() != nil || dependencies.Validate() != nil {
		return fmt.Errorf("%w: reviewed input requires exact original read dependencies", common.ErrConflict)
	}
	for _, file := range input.InputReview.Files {
		matches := 0
		for _, read := range dependencies.Entries {
			if read.OwnerKind == "capability" && read.OwnerInstanceID == ownerInstance && read.ResourceKind == string(runtimecontract.RuntimeResourceWorkspaceTarget) &&
				read.ResourceID == file.TargetRef && read.Revision == file.ContentHash {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%w: reviewed input bytes are absent or ambiguous in original validation owner reads", common.ErrConflict)
		}
	}
	return nil
}

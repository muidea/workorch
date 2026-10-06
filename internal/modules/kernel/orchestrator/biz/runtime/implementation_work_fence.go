package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// validateRuntimeCandidatePurposeFence selects no action. It validates the
// already-reduced purpose without lending terminal CWI bindings to ordinary work.
func validateRuntimeCandidatePurposeFence(a RuntimeAggregate) error {
	if a.Decision.RequiredOutput.ImplementationWorkBindingHash == "" {
		return validateCurrentCWIBindingFence(a.Facts)
	}
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(a)
	if err != nil {
		return err
	}
	_, err = runtimecontract.ValidateRuntimeImplementationWorkAdmission(a.Facts, a.Decision, a.SemanticPlanning, a.ImplementationWork, manifest)
	return err
}

// validateRuntimeExecutionPurposeFence consumes only the immutable active seal.
// Partial receipt invalidation does not revoke remaining sealed operations, but
// independent original semantic/round/dispatch source joins remain mandatory.
func validateRuntimeExecutionPurposeFence(a RuntimeAggregate) error {
	if a.ActiveDispatch == nil || a.ActiveDispatch.ImplementationWorkCommitment == nil {
		return validateCurrentCWIBindingFenceForEvidenceCommit(a.Facts)
	}
	if err := runtimecontract.ValidateRuntimeCWILedger(a.Facts); err != nil {
		return err
	}
	dispatch := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
	if err := runtimecontract.ValidateRuntimeImplementationWorkDispatchCommitment(dispatch); err != nil {
		return err
	}
	if a.Decision.RequiredOutput.ImplementationWorkBindingHash != dispatch.ImplementationWorkCommitment.WorkingBinding.Hash || a.Facts.Failure != nil || a.Facts.RepairDirective != nil {
		return fmt.Errorf("%w: work execution lost exact ordinary-work authority", common.ErrConflict)
	}
	return runtimecontract.ValidateRuntimeImplementationWorkState(a.Definition, a.Facts, a.ImplementationWork, a.SemanticPlanning, &dispatch, sealedDispatchesToRuntimeRecords(a.CompletedDispatches), a.OperationReceipts, a.OperationGovernanceBindings, a.ExecutionBudget)
}

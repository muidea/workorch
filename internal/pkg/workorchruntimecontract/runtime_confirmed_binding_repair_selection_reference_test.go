package workorchruntimecontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"slices"
)

// Complete frozen pre-change confirmed-repair consumer; test-only independent oracle.
func referenceConfirmedRepairBeforeCandidateSelection(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, cutoff string, session *runtimeHistoricalPreimageSession) bool {
	// Currentness is a necessary condition of this consumer. An old binding
	// cannot consume a repair, regardless of the unrelated source graph. Reject
	// it before rebuilding that graph; every possible success still traverses
	// the complete original guards below. This is not retained proof/credit.
	current := 0
	for _, item := range sources.Facts.Coverage {
		if item.ID == binding.CoverageWorkItemID && item.CurrentBindingRef != nil && runtimeLineageBindingMatches(binding, item.ID, *item.CurrentBindingRef) {
			current++
		}
	}
	if current != 1 {
		return false
	}
	if binding.Validate() != nil || !runtimeCanonicalLineageCutoff(cutoff) ||
		sources.Budget.Validate() != nil ||
		runtimeValidateLineageOperationsInSession(sources, session) != nil ||
		ValidateRuntimeRepairSurfaceConfirmationRecords(sources.Confirmations) != nil ||
		!runtimeFreshBindingInputReviewInSession(binding, sources.Semantic, session) {
		return false
	}
	for _, repair := range sources.Completed {
		if repair.RepairCommitment == nil || repair.State != RuntimeDispatchStateCommitted || repair.Candidate.StateChange == nil || repair.Candidate.Validation != nil || repair.ImplementationWorkCommitment != nil || !RuntimeCoverageEvidenceBindingProjectionsEqual(repair.CoverageBindings, []CoverageEvidenceBinding{binding}) {
			continue
		}
		c := *repair.RepairCommitment
		// This first bounded version supports the exact original dispatch, not
		// a partial/governance-resume set assembled across different dispatches.
		if ValidateRuntimeDispatchRepairCommitmentProjection(repair.RepairCommitmentHash, repair.RepairCommitment) != nil || c.InputRepair != nil || c.OriginDispatchID != repair.ID || c.FocusID != repair.Transaction.FocusID || !slices.Equal(c.Closure.CoverageWorkItemIDs, []common.CoverageWorkItemID{binding.CoverageWorkItemID}) {
			continue
		}
		rejected, receipt, operation, ok := runtimeConfirmedBindingRepairSource(binding, sources, repair)
		if !ok {
			continue
		}
		diagnostic := *receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic
		var source RuntimeCoverageItem
		for _, item := range sources.Facts.DefinitionSeed.CoverageItems {
			if item.ID == binding.CoverageWorkItemID {
				source = item
			}
		}
		policy, err := CompileRuntimeSourceEvidencePolicy(source.SourceScope, source.SourceID, sources.Facts.Activation.ExecutionContract.ProfileContracts, nil)
		descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
		if err != nil || !known || policy.ValidationFailureRepairConvergence == nil {
			continue
		}
		route, found := policy.ValidationFailureRepairConvergence.RouteForTrigger(descriptor.OwnerModuleID, operation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode)
		if !found || !route.RequiresStructuredObservation() || route.RepairOwnerModuleID != c.RepairOwnerModuleID || route.RepairTargetPolicy != RuntimeOperationTargetPolicyExistingOwnerScope && route.RepairTargetPolicy != RuntimeOperationTargetPolicyExactTarget {
			continue
		}
		confirmationMatches := 0
		for _, record := range sources.Confirmations {
			if record.IdentitySignature != c.ConfirmationIdentityHash || record.Signature != c.ConfirmationHash {
				continue
			}
			confirmation, observation := record.Confirmation, record.Confirmation.Observation
			if confirmation.Disposition != RuntimeRepairSurfaceConfirmed || confirmation.OwnerModuleID != c.RepairOwnerModuleID || !slices.Equal(confirmation.ConfirmedTargetRefs, c.Closure.GroundedTargetRefs) || observation.WorkspaceID != sources.Facts.WorkspaceID || observation.RunID != sources.Facts.RunID || observation.DispatchID != rejected.DispatchID || observation.OperationID != rejected.OperationID || observation.AuthoritySnapshotID != rejected.AuthoritySnapshotID || observation.FailureSignature != rejected.FailureSignature || observation.DiagnosticClass != diagnostic.Class || !slices.Equal(observation.CandidateTargetRefs, diagnostic.TargetRefs) || observation.DiagnosticHash != StableRuntimeValidationFailureObservationDiagnosticHash(diagnostic, observation.ValidationScopeTargetRefs) {
				return false
			}
			confirmationMatches++
		}
		if confirmationMatches != 1 {
			continue
		}
		operations := runtimeCandidateAdmissionOperations(repair.Candidate)
		if len(operations) == 0 || len(repair.CompletedOperationIDs) != len(operations) {
			continue
		}
		last, complete := rejected.Revision, true
		for _, member := range operations {
			owner, known := RuntimeOperationDescriptorForKind(member.Kind)
			fact, receipt, valid := runtimeLineageOperationFactFromLedgers(repair, member, sources, true, true, false)
			if !known || owner.OwnerModuleID != c.RepairOwnerModuleID || !slices.Contains(route.RepairOperationKinds, member.Kind) || !valid || !receipt.Changed || !RuntimeEffectIsDurable(receipt.ObservedEffectScope) || !slices.Equal(member.BindingRefs, []string{binding.ID}) || len(receipt.TargetRefs) == 0 || !runtimeTargetRefsSubset(receipt.TargetRefs, c.Closure.GroundedTargetRefs) || !EvidenceRevisionAfter(fact.Revision, rejected.Revision) || EvidenceRevisionAfter(fact.Revision, cutoff) {
				complete = false
				break
			}
			if EvidenceRevisionAfter(fact.Revision, last) {
				last = fact.Revision
			}
		}
		if complete && runtimeConfirmedBindingFreshValidation(binding, sources, c, last, cutoff) {
			return true
		}
	}
	return false
}

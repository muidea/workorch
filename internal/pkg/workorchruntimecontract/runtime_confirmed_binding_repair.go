package workorchruntimecontract

import (
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Pure versioned consumer of an existing ordinary binding's confirmed repair.
// No ledger mutation, retrospective credit, source selection or effect replay.
func runtimeConfirmedBindingRepairSatisfied(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, cutoff string) bool {
	return runtimeConfirmedBindingRepairInSession(binding, sources, cutoff, nil)
}

func runtimeConfirmedBindingRepairInSession(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, cutoff string, session *runtimeHistoricalPreimageSession) bool {
	if binding.Validate() != nil || !runtimeCanonicalLineageCutoff(cutoff) || sources.Budget.Validate() != nil || runtimeValidateLineageOperationsInSession(sources, session) != nil || validateRuntimeRepairSurfaceConfirmationRecordsInIndex(sources.Confirmations, session.lineageIndex()) != nil || !runtimeFreshBindingInputReviewInSession(binding, sources.Semantic, session) {
		return false
	}
	current := 0
	for _, item := range sources.Facts.Coverage {
		if item.ID == binding.CoverageWorkItemID && item.CurrentBindingRef != nil && runtimeLineageBindingMatches(binding, item.ID, *item.CurrentBindingRef) {
			current++
		}
	}
	if current != 1 {
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
		rejected, receipt, operation, ok := runtimeConfirmedBindingRepairSourceInIndex(binding, sources, repair, session.lineageIndex())
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
			fact, receipt, valid := runtimeLineageOperationFactFromLedgersInIndex(repair, member, sources, true, true, false, session.lineageIndex())
			if !known || owner.OwnerModuleID != c.RepairOwnerModuleID || !slices.Contains(route.RepairOperationKinds, member.Kind) || !valid || !receipt.Changed || !RuntimeEffectIsDurable(receipt.ObservedEffectScope) || !slices.Equal(member.BindingRefs, []string{binding.ID}) || len(receipt.TargetRefs) == 0 || !runtimeTargetRefsSubset(receipt.TargetRefs, c.Closure.GroundedTargetRefs) || !EvidenceRevisionAfter(fact.Revision, rejected.Revision) || EvidenceRevisionAfter(fact.Revision, cutoff) {
				complete = false
				break
			}
			if EvidenceRevisionAfter(fact.Revision, last) {
				last = fact.Revision
			}
		}
		if complete && runtimeConfirmedBindingFreshValidationInIndex(binding, sources, c, last, cutoff, session.lineageIndex()) {
			return true
		}
	}
	return false
}

func runtimeConfirmedBindingRepairSource(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, repair RuntimeDispatchRecord) (EvidenceFact, RuntimeOperationReceipt, RuntimeOperation, bool) {
	return runtimeConfirmedBindingRepairSourceInIndex(binding, sources, repair, nil)
}

func runtimeConfirmedBindingRepairSourceInIndex(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, repair RuntimeDispatchRecord, index *runtimeOwnerLineageIndex) (EvidenceFact, RuntimeOperationReceipt, RuntimeOperation, bool) {
	c := *repair.RepairCommitment
	var fact EvidenceFact
	var receipt RuntimeOperationReceipt
	var operation RuntimeOperation
	matches := 0
	for _, dispatch := range sources.Completed {
		if dispatch.ID != c.SourceDispatchID || dispatch.RepairCommitment != nil || !RuntimeCoverageEvidenceBindingProjectionsEqual(dispatch.CoverageBindings, []CoverageEvidenceBinding{binding}) {
			continue
		}
		for _, member := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
			if member.ID != c.SourceOperationID {
				continue
			}
			f, r, valid := runtimeLineageOperationFactFromLedgersInIndex(dispatch, member, sources, false, false, true, index)
			if !valid || f.Kind != EvidenceKindValidation || f.ID != c.SourceEvidenceFactID || StableRuntimeEvidenceFactHash(f) != c.SourceEvidenceFactHash || f.AuthoritySnapshotID != c.SourceAuthoritySnapshotID || StableRuntimeOperationHash(member) != c.SourceOperationHash || r.ExecutionKey != c.SourceReceiptExecutionKey || r.Fence != c.SourceReceiptFence || index.receiptHash(r) != c.SourceReceiptHash || f.Revision != c.Closure.BaselineEvidenceRevision || f.Revision != repair.Transaction.EvidenceRevision || f.FailureSignature != c.Closure.GapSignature || len(f.CoverageWorkItemIDs) != 0 && !slices.Equal(f.CoverageWorkItemIDs, c.Closure.CoverageWorkItemIDs) || !slices.Equal(f.BindingRefs, []string{binding.ID}) || r.Feedback == nil || r.Feedback.OriginalDiagnostic == nil || r.Feedback.OriginalDiagnostic.ValidationDiagnostic == nil {
				continue
			}
			fact, receipt, operation, matches = f, r, member, matches+1
		}
	}
	return fact, receipt, operation, matches == 1
}

func runtimeConfirmedBindingFreshValidation(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, c RuntimeDispatchRepairCommitment, repairRevision, cutoff string) bool {
	return runtimeConfirmedBindingFreshValidationInIndex(binding, sources, c, repairRevision, cutoff, nil)
}

func runtimeConfirmedBindingFreshValidationInIndex(binding CoverageEvidenceBinding, sources RuntimeLineageAssertionSources, c RuntimeDispatchRepairCommitment, repairRevision, cutoff string, index *runtimeOwnerLineageIndex) bool {
	proof, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	if !found {
		return false
	}
	for _, dispatch := range sources.Completed {
		if dispatch.State != RuntimeDispatchStateCommitted || dispatch.Candidate.Validation == nil || dispatch.RepairCommitment != nil || dispatch.ImplementationWorkCommitment != nil || !RuntimeCoverageEvidenceBindingProjectionsEqual(dispatch.CoverageBindings, []CoverageEvidenceBinding{binding}) {
			continue
		}
		manifests := 0
		for _, session := range sources.Sessions {
			if session.CandidateManifest != nil && session.Transaction == dispatch.Transaction && session.CandidateManifest.Hash == dispatch.CandidateManifestHash && session.CandidateManifest.RepairClosureID == c.Closure.ID && session.CandidateManifest.RepairPurpose == "" && RuntimeCoverageEvidenceBindingProjectionsEqual(session.CandidateManifest.Bindings, []CoverageEvidenceBinding{binding}) {
				manifests++
			}
		}
		if manifests != 1 {
			continue
		}
		operations := runtimeCandidateAdmissionOperations(dispatch.Candidate)
		if len(operations) == 0 || len(dispatch.CompletedOperationIDs) != len(operations) {
			continue
		}
		var accepted []EvidenceFact
		last, complete := repairRevision, true
		for _, member := range operations {
			fact, receipt, valid := runtimeLineageOperationFactFromLedgersInIndex(dispatch, member, sources, true, false, false, index)
			if !valid || MatchCoverageEvidenceBindingReceipt(binding, EvidenceKindValidation, member, receipt) != nil || fact.Kind != EvidenceKindValidation || !slices.Equal(fact.BindingRefs, []string{binding.ID}) || !slices.Equal(fact.CoverageWorkItemIDs, c.Closure.CoverageWorkItemIDs) || !EvidenceRevisionAfter(fact.Revision, repairRevision) || EvidenceRevisionAfter(fact.Revision, cutoff) || fact.Validation == nil || !RuntimeValidationObservationSatisfiesCoverageProof(proof, *fact.Validation) {
				complete = false
				break
			}
			accepted = append(accepted, fact)
			if EvidenceRevisionAfter(fact.Revision, last) {
				last = fact.Revision
			}
		}
		// TargetRefs is the finite authorized existing-owner scope, not an
		// all-target operation obligation. Each receipt above already matches the
		// exact binding, operation and ordinary all-of recipe. Only explicitly
		// declared prospective targets require complete per-target membership.
		if !complete || !runtimeConfirmedBindingValidationTargetsComplete(binding, accepted) {
			continue
		}
		if !runtimeConfirmedBindingHasLaterFailure(binding, sources.Facts.Evidence.Rejected, last, cutoff) {
			return true
		}
	}
	return false
}

func runtimeConfirmedBindingValidationTargetsComplete(binding CoverageEvidenceBinding, accepted []EvidenceFact) bool {
	return len(accepted) != 0 && assessmentProspectiveTargetsComplete(binding.ProspectiveTargetRefs, accepted)
}

func runtimeConfirmedBindingHasLaterFailure(binding CoverageEvidenceBinding, rejected []EvidenceFact, after, cutoff string) bool {
	for _, fact := range rejected {
		// A rejected validation may have no CWI credit at all. Its exact
		// ordinary binding still identifies the source whose fresh proof failed.
		if fact.Kind == EvidenceKindValidation && (slices.Contains(fact.BindingRefs, binding.ID) || slices.Contains(fact.CoverageWorkItemIDs, binding.CoverageWorkItemID)) && EvidenceRevisionAfter(fact.Revision, after) && !EvidenceRevisionAfter(fact.Revision, cutoff) {
			return true
		}
	}
	return false
}

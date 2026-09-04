package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/proof"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// recordValidationFailureObservation is intentionally an identity fence, not
// a state transition. The receipt/evidence transaction is the only producer
// of the validation-owner fact; this method makes the following resource-owner
// handoff explicit and rejects stale/forged observations before any owner query.
func (c *authorityComponent) recordValidationFailureObservation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, observation runtimecontract.RuntimeValidationFailureObservation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || observation.Validate() != nil || observation.WorkspaceID != workspaceID || observation.RunID != runID {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Facts.Failure == nil || aggregate.Facts.Failure.ValidationFailureObservation == nil || !runtimecontract.SameRuntimeValidationFailureObservation(*aggregate.Facts.Failure.ValidationFailureObservation, observation) || !sealedValidationFailureObservationMatches(aggregate, observation) || !runtimeValidationFailureObservationHasPersistedReceipt(aggregate, observation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: validation failure observation does not match persisted receipt", common.ErrConflict)
	}
	return aggregate, nil
}

// recordRepairSurfaceConfirmation is the sole SM-2 transition from a typed
// resource-owner fact to repair authority. The resource owner cannot select
// the action, and the validation owner cannot promote diagnostic candidates
// without this exact confirmation.
func (c *authorityComponent) recordRepairSurfaceConfirmation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || confirmation.Validate() != nil || confirmation.Observation.WorkspaceID != workspaceID || confirmation.Observation.RunID != runID {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	confirmation = confirmation.Normalized()
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Persisted typed confirmation records are checked before terminal/failure
	// guards.  This makes exact replay a true no-op even after restart or after
	// the first confirmation has retired the mutable target pointers.
	if repairSurfaceConfirmationExists(aggregate, confirmation) {
		return aggregate, nil
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(aggregate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !plan.Required() {
		// Preserve the existing terminal diagnostics while making the current
		// owner-private plan the admission authority for every new response.
		// In particular, a response from an earlier read cannot cross a stale
		// confirmation's prerequisite-reacquisition interlude.
		if aggregate.Facts.Resources.FailureTarget != nil {
			return RuntimeAggregate{}, fmt.Errorf("%w: repair surface observation already has terminal target authority", common.ErrConflict)
		}
		if aggregate.Decision.State == runtimecontract.ConvergenceStateComplete || aggregate.Decision.State == runtimecontract.ConvergenceStateFailed {
			return RuntimeAggregate{}, fmt.Errorf("%w: terminal runtime cannot accept a new repair surface confirmation", common.ErrConflict)
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime authority does not require repair surface confirmation", common.ErrConflict)
	}
	// The concrete typed adapter selected by plan.Protocol binds the response
	// protocol before this owner-private event is submitted. Re-deriving that
	// protocol from the fresh aggregate here prevents a delayed response from
	// crossing a changed sealed route without adding protocol state to the
	// confirmation DTO.
	if plan.Protocol == runtimecontract.RuntimeRepairSurfaceConfirmationProtocolNone ||
		!runtimecontract.RuntimeRepairSurfaceConfirmationProtocolSupported(plan.Protocol) {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation has no current protocol", common.ErrConflict)
	}
	if !runtimecontract.SameRuntimeValidationFailureObservation(plan.Observation, confirmation.Observation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation does not match current observation", common.ErrConflict)
	}
	if confirmation.RequestID != plan.RequestID || confirmation.IssuedAtStateRevision != plan.IssuedAtStateRevision {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation request does not match current confirmation window", common.ErrConflict)
	}
	if confirmation.OwnerModuleID != plan.OwnerModuleID {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation owner does not match sealed repair provenance", common.ErrConflict)
	}
	if confirmation.SubjectRef != plan.SubjectRef {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation subject does not match current protocol", common.ErrConflict)
	}
	if confirmation.ExpectedSubjectRevision != plan.ExpectedSubjectRevision {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation expected revision does not match current owner fact", common.ErrConflict)
	}
	if repairSurfaceConfirmationIdentityConflict(aggregate, confirmation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation conflicts with persisted disposition", common.ErrConflict)
	}
	failure := aggregate.Facts.Failure
	if failure == nil || failure.ValidationFailureObservation == nil || !runtimecontract.SameRuntimeValidationFailureObservation(*failure.ValidationFailureObservation, confirmation.Observation) || !sealedValidationFailureObservationMatches(aggregate, confirmation.Observation) || !runtimeValidationFailureObservationHasPersistedReceipt(aggregate, confirmation.Observation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation does not match persisted validation failure", common.ErrConflict)
	}
	if aggregate.Facts.Resources.FailureTarget != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface observation already has terminal target authority", common.ErrConflict)
	}
	expected := aggregate.StateRevision
	if confirmation.Disposition == runtimecontract.RuntimeRepairSurfaceRejected {
		updatedFailure := failure.Clone()
		// A rejected owner surface is a typed infeasibility fact, not evidence
		// that the repair budget was consumed.  Keep the terminal code stable for
		// callers while preserving the original diagnostic nested in the failure.
		updatedFailure.Class = runtimecontract.FailureClassCandidateInfeasible
		updatedFailure.Code = "repair_surface_unavailable"
		updatedFailure.Message = firstNonEmpty(strings.TrimSpace(confirmation.Reason), "resource owner could not confirm a repair surface")
		updatedFailure.TargetRefs = nil
		updatedFailure.TargetObservation = nil
		if err := replaceRuntimeFailureAuthority(&aggregate.Facts, updatedFailure, runtimeFailureAuthorityRetire, nil); err != nil {
			return RuntimeAggregate{}, err
		}
		aggregate.Facts.Resources.TargetSelection = nil
		aggregate.Facts.Resources.TargetSnapshots = nil
	} else if confirmation.Disposition == runtimecontract.RuntimeRepairSurfaceStale {
		staleObservation, found := currentRuntimeRepairSurfaceSubjectObservation(
			aggregate,
			confirmation.OwnerModuleID,
			plan.SubjectDomain,
			confirmation.SubjectRef,
			confirmation.ExpectedSubjectRevision,
		)
		if !found {
			return RuntimeAggregate{}, fmt.Errorf("%w: stale repair surface has no exact current prerequisite observation", common.ErrConflict)
		}
		stale := runtimecontract.RuntimePrerequisiteStale{
			PrerequisiteID:   staleObservation.PrerequisiteID,
			ObservationID:    staleObservation.ObservationID,
			Domain:           plan.SubjectDomain,
			OwnerModuleID:    confirmation.OwnerModuleID,
			SubjectRef:       confirmation.SubjectRef,
			ExpectedRevision: confirmation.ExpectedSubjectRevision,
			ObservedRevision: confirmation.ObservedRevision,
			Reason: firstNonEmpty(
				strings.TrimSpace(confirmation.Reason),
				"resource subject changed before repair-surface confirmation",
			),
		}
		stale = stale.Normalized()
		if err := stale.Validate(); err != nil {
			return RuntimeAggregate{}, err
		}
		updatedFailure := failure.Clone()
		// Stale owner scope is a freshness fact. Retire every current target
		// pointer and let the normal SM-2 prerequisite compiler reacquire the
		// sealed focus scope; do not turn a stale observation into a terminal.
		updatedFailure.TargetRefs = nil
		updatedFailure.TargetObservation = nil
		if err := replaceRuntimeFailureAuthority(&aggregate.Facts, updatedFailure, runtimeFailureAuthorityRetire, nil); err != nil {
			return RuntimeAggregate{}, err
		}
		aggregate.Facts.Resources.TargetSelection = nil
		aggregate.Facts.Resources.TargetSnapshots = nil
		facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
		if err != nil {
			return RuntimeAggregate{}, err
		}
		recovery, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
		if err != nil {
			return RuntimeAggregate{}, err
		}
		facts.Recovery = recovery
		staleCopy := stale
		aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
			Signature:       runtimecontract.StableRuntimePrerequisiteStaleSignature(stale),
			PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
			InvalidatedDependencies: invalidatedDependencies, Stale: &staleCopy,
		})
		if !consumed {
			exhausted := runtimecontract.FailureFact{
				Class:     runtimecontract.FailureClassPrerequisiteRefreshExhausted,
				Code:      "prerequisite_refresh_exhausted",
				Signature: runtimecontract.StableRuntimePrerequisiteStaleSignature(stale),
				Message:   "prerequisite refresh budget exhausted: " + stale.Reason,
			}
			if err := replaceRuntimeFailureAuthority(&facts, exhausted, runtimeFailureAuthorityRetire, nil); err != nil {
				return RuntimeAggregate{}, err
			}
		} else {
			facts = preflightRuntimeFactsPreservingPending(facts, runtimeTargetGroundingRequirements(facts))
		}
		aggregate.Facts = facts
	} else {
		updatedFailure := failure.Clone()
		updatedFailure.TargetRefs = append([]string(nil), confirmation.ConfirmedTargetRefs...)
		updatedFailure.TargetObservation = &runtimecontract.RuntimeFailureTargetObservation{OwnerModuleID: confirmation.OwnerModuleID, SubjectRef: confirmation.SubjectRef, TargetRole: confirmation.Observation.TargetRole, ObservedRevision: confirmation.ObservedRevision, TargetRefs: confirmation.ConfirmedTargetRefs}
		ownerTarget := &runtimecontract.FailureTarget{Signature: updatedFailure.Signature, TargetRefs: append([]string(nil), confirmation.ConfirmedTargetRefs...), TargetObservation: updatedFailure.TargetObservation}
		if err := replaceRuntimeFailureAuthority(&aggregate.Facts, updatedFailure, runtimeFailureAuthorityReplaceFromExactOwnerFact, ownerTarget); err != nil {
			return RuntimeAggregate{}, err
		}
		// A newly confirmed owner surface replaces stale focus snapshots before
		// issuing the state-change authority.
		aggregate.Facts = preflightRuntimeFactsPreservingPending(aggregate.Facts, runtimeTargetGroundingRequirements(aggregate.Facts))
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	aggregate.StateRevision++
	record, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(confirmation)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if record.RecordedAtStateRevision != aggregate.StateRevision {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation record revision does not match its owner transition", common.ErrConflict)
	}
	aggregate.RepairSurfaceConfirmations = append(aggregate.RepairSurfaceConfirmations, record)
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expected); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// currentRuntimeRepairSurfaceSubjectObservation resolves a deterministic
// current prerequisite observation for the exact owner revision named by the
// read-only plan. invalidatePrerequisiteObservationsForStale uses that one
// identity to retire every current observation sharing the dependency.
func currentRuntimeRepairSurfaceSubjectObservation(aggregate RuntimeAggregate, ownerModuleID string, subjectDomain runtimecontract.RuntimeResourceDomain, subjectRef, revision string) (runtimecontract.RuntimePrerequisiteObservation, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	subjectRef = strings.TrimSpace(subjectRef)
	revision = strings.TrimSpace(revision)
	var selected runtimecontract.RuntimePrerequisiteObservation
	for _, rawObservation := range aggregate.Facts.Resources.PrerequisiteObservations {
		observation := rawObservation.Normalized()
		if observation.OwnerModuleID != ownerModuleID {
			continue
		}
		matched := false
		for _, rawDependency := range observation.Dependencies {
			dependency := rawDependency.Normalized()
			if dependency.Domain == subjectDomain && dependency.OwnerModuleID == ownerModuleID &&
				dependency.SubjectRef == subjectRef && dependency.Revision == revision {
				matched = true
				break
			}
		}
		if !matched || (selected.PrerequisiteID != "" && selected.PrerequisiteID <= observation.PrerequisiteID) {
			continue
		}
		selected = observation
	}
	return selected, selected.PrerequisiteID != ""
}

func repairSurfaceConfirmationExists(aggregate RuntimeAggregate, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) bool {
	return proof.RepairSurfaceConfirmationExists(aggregate.RepairSurfaceConfirmations, confirmation)
}

func repairSurfaceConfirmationIdentityConflict(aggregate RuntimeAggregate, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) bool {
	return proof.RepairSurfaceConfirmationIdentityConflict(aggregate.RepairSurfaceConfirmations, confirmation)
}

// sealedValidationFailureObservationMatches binds the diagnostic candidates
// back to the immutable validation operation that produced them.  This is the
// Orchestrator-side causation check; the resource owner subsequently verifies the current
// resource revision. Neither check interprets diagnostic text.
func sealedValidationFailureObservationMatches(aggregate RuntimeAggregate, observation runtimecontract.RuntimeValidationFailureObservation) bool {
	if observation.WorkspaceID != aggregate.Facts.WorkspaceID || observation.RunID != aggregate.Facts.RunID {
		return false
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.State == DispatchStateCommitted && containsString(dispatch.CompletedOperationIDs, observation.OperationID) &&
			sealedValidationFailureObservationMatchesDispatch(dispatch, observation) {
			return true
		}
	}
	return false
}

// sealedValidationFailureObservationMatchesDispatch binds an observation to
// one immutable dispatch seal. It is shared by receipt admission (active
// dispatch) and post-execution SM-2 confirmation (completed dispatch), so both
// boundaries apply the same per-operation authorization semantics.
func sealedValidationFailureObservationMatchesDispatch(dispatch SealedDispatch, observation runtimecontract.RuntimeValidationFailureObservation) bool {
	if dispatch.Candidate.Validation == nil {
		return false
	}
	operations := make([]proof.SealedValidationOperation, 0, len(dispatch.Candidate.Validation.Operations))
	for _, operation := range dispatch.Candidate.Validation.Operations {
		value := operation.Operation.Normalized()
		targetAuthorization, err := runtimeValidationTargetAuthorization(dispatch, value)
		if err != nil {
			continue
		}
		operations = append(operations, proof.SealedValidationOperation{
			DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
			OperationID: value.ID, TargetRole: dispatch.FocusTargetRole,
			OperationTargetRefs: append([]string(nil), value.TargetRefs...),
			RequireTargets:      targetAuthorization.RequireTargets,
			TargetAuthorization: targetAuthorization.Target,
		})
	}
	return proof.ValidationFailureObservationMatchesSealedOperation(operations, observation)
}

// runtimeValidationFailureObservationHasPersistedReceipt proves provenance,
// not merely shape: the exact observation must be nested in the rejected
// receipt for the sealed validation operation it names. The operation
// descriptor, rather than a profile-specific owner constant, identifies the
// capability owner whose diagnostic may be admitted.
func runtimeValidationFailureObservationHasPersistedReceipt(aggregate RuntimeAggregate, observation runtimecontract.RuntimeValidationFailureObservation) bool {
	_, found := runtimeValidationFailurePersistedDiagnostic(aggregate, observation)
	return found
}

// runtimeValidationFailurePersistedDiagnostic resolves the exact immutable
// validation-owner fact behind an observation. Historical repair confirmation
// invariants must use this receipt lineage: the mutable SM-2 Failure is
// intentionally retired after repair and therefore cannot remain provenance.
func runtimeValidationFailurePersistedDiagnostic(aggregate RuntimeAggregate, observation runtimecontract.RuntimeValidationFailureObservation) (runtimecontract.ValidationFailureDiagnostic, bool) {
	var completed *SealedDispatch
	for idx := range aggregate.CompletedDispatches {
		dispatch := &aggregate.CompletedDispatches[idx]
		if dispatch.ID == observation.DispatchID && dispatch.State == DispatchStateCommitted &&
			containsString(dispatch.CompletedOperationIDs, observation.OperationID) {
			completed = dispatch
			break
		}
	}
	if completed == nil || !sealedValidationFailureObservationMatchesDispatch(*completed, observation) {
		return runtimecontract.ValidationFailureDiagnostic{}, false
	}
	operation, found := candidateOperationForEvidence(completed.Candidate, observation.OperationID)
	if !found {
		return runtimecontract.ValidationFailureDiagnostic{}, false
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || descriptor.FailureTargetProtocol != runtimecontract.RuntimeFailureTargetProtocolValidationCandidates {
		return runtimecontract.ValidationFailureDiagnostic{}, false
	}
	execution, found := operationExecutionRecord(completed.OperationExecutions, observation.OperationID)
	if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.Fence == 0 {
		return runtimecontract.ValidationFailureDiagnostic{}, false
	}
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.OperationID != observation.OperationID ||
		execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) ||
		receipt.Feedback == nil || receipt.Feedback.WorkspaceID != observation.WorkspaceID || receipt.Feedback.RunID != observation.RunID ||
		receipt.Feedback.DispatchID != observation.DispatchID || receipt.Feedback.AuthoritySnapshotID != observation.AuthoritySnapshotID ||
		receipt.Feedback.OriginalDiagnostic == nil || receipt.Feedback.OriginalDiagnostic.OwnerModuleID != descriptor.OwnerModuleID ||
		receipt.Feedback.OriginalDiagnostic.ValidationFailureObservation == nil || receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic == nil {
		return runtimecontract.ValidationFailureDiagnostic{}, false
	}
	if !runtimecontract.SameRuntimeValidationFailureObservation(*receipt.Feedback.OriginalDiagnostic.ValidationFailureObservation, observation) {
		return runtimecontract.ValidationFailureDiagnostic{}, false
	}
	diagnostic := receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic.Clone()
	if runtimecontract.StableRuntimeValidationFailureObservationDiagnosticHash(diagnostic, observation.ValidationScopeTargetRefs) != observation.DiagnosticHash {
		return runtimecontract.ValidationFailureDiagnostic{}, false
	}
	return diagnostic, true
}

package runtime

import (
	"context"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func finalizeExecutionEvidence(aggregate *RuntimeAggregate, admission executionEvidenceAdmission) (bool, error) {
	evidence := admission.Evidence
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
	dispatch := *aggregate.ActiveDispatch
	dispatch.CompletedOperationIDs = appendNormalizedString(dispatch.CompletedOperationIDs, evidence.OperationID)

	prerequisitesInvalidated := false
	var err error
	if evidence.Status == ExecutionEvidenceStatusAccepted && evidence.Receipt != nil {
		// Changed owner facts affect the next SM-2 authority, not the immutable
		// operation set already sealed by this SM-3 transaction.
		prerequisitesInvalidated, err = applyReceiptResourceChangeInvalidation(aggregate, evidence.DispatchID, evidence.OperationID, *evidence.Receipt)
		if err != nil {
			return false, err
		}
	}
	dispatchInvalidated := evidenceDispatchInvalidated(*aggregate, evidence, prerequisitesInvalidated)
	dispatchCompleted := dispatchInvalidated || evidence.Status == ExecutionEvidenceStatusRejected || len(dispatch.CompletedOperationIDs) == candidateOperationCount(dispatch.Candidate)
	if !dispatchCompleted {
		aggregate.ActiveDispatch = &dispatch
		return false, nil
	}
	if err := completeEvidenceDispatch(aggregate, dispatch, dispatchInvalidated); err != nil {
		return false, err
	}
	return true, nil
}

func evidenceDispatchInvalidated(aggregate RuntimeAggregate, evidence ExecutionEvidence, prerequisitesInvalidated bool) bool {
	effectUncommittable := evidence.Status == ExecutionEvidenceStatusRejected && evidence.Failure != nil &&
		evidence.Failure.Class == runtimecontract.FailureClassEffectMismatch && evidence.Receipt != nil &&
		(evidence.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted || evidence.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp)
	// Exhausting the bounded refresh ledger is a determined failure. Ordinary
	// prerequisite invalidation is not.
	refreshExhausted := prerequisitesInvalidated && aggregate.Facts.Failure != nil &&
		aggregate.Facts.Failure.Class == runtimecontract.FailureClassPrerequisiteRefreshExhausted
	return effectUncommittable || refreshExhausted
}

func completeEvidenceDispatch(aggregate *RuntimeAggregate, dispatch SealedDispatch, invalidated bool) error {
	if invalidated {
		dispatch.State = DispatchStateInvalidated
	} else {
		dispatch.State = DispatchStateCommitted
	}
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	if err := retireSemanticPlanningForCurrentReopenedCWIs(aggregate); err != nil {
		return err
	}

	// A completed transaction remains durable audit evidence, but SM-2 must
	// reacquire changed owner facts before advancing its focus.
	canAdvanceFocus := !invalidated && len(aggregate.Facts.Prerequisites.Pending) == 0
	previousFocusID := aggregate.Facts.Focus.ID
	if canAdvanceFocus {
		advanced, err := advanceRuntimeCWIFocus(aggregate.Facts, aggregate.CoverageProofLineage)
		if err != nil {
			return err
		}
		aggregate.Facts = advanced
		if aggregate.Facts.Focus.ID != previousFocusID {
			if err := prepareSemanticPlanningForAdvancedFocus(aggregate); err != nil {
				return err
			}
		}
	}
	var previousSnapshot *runtimecontract.AuthoritySnapshot
	if canAdvanceFocus {
		previousSnapshot = &aggregate.Decision.AuthoritySnapshot
	}
	facts, nextDecision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, previousSnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return err
	}
	aggregate.Facts = facts
	aggregate.Decision = nextDecision
	return nil
}

func recordStaleExecutionEvidence(aggregate *RuntimeAggregate, evidence ExecutionEvidence, currentSnapshotID string) {
	aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
	aggregate.Diagnostics = append(aggregate.Diagnostics, runtimecontract.RuntimeDiagnosticRecord{
		EventID: evidence.EventID, AuthoritySnapshotID: evidence.AuthoritySnapshotID, CurrentSnapshotID: currentSnapshotID,
		DispatchID: evidence.DispatchID, Reason: "execution evidence belongs to a stale authority snapshot",
	})
}

func (c *candidateDispatchComponent) commitEvidenceTransition(ctx context.Context, aggregate RuntimeAggregate, expectedRevision uint64, disposition CommitDisposition, dispatchCompleted bool) (EvidenceCommitResult, error) {
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return EvidenceCommitResult{}, err
	}
	return EvidenceCommitResult{
		Disposition:       disposition,
		DispatchCompleted: dispatchCompleted,
		Aggregate:         aggregate,
	}, nil
}

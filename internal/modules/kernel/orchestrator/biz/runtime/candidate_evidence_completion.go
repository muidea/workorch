package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func finalizeExecutionEvidence(
	aggregate *RuntimeAggregate,
	admission executionEvidenceAdmission,
	fact runtimecontract.EvidenceFact,
) (bool, error) {
	evidence := admission.Evidence
	if evidence.Status == ExecutionEvidenceStatusRejected {
		// Rejection admission enriches the durable fact with the receipt-owned
		// failure signature/class before appending it. Use that exact persisted
		// value for subsequent private SM-2 transitions; the pre-adjudication
		// local copy deliberately contains no diagnostic authority.
		persisted, found := runtimeRejectedEvidenceFactForEvent(aggregate.Facts.Evidence.Rejected, evidence.EventID)
		if !found {
			return false, fmt.Errorf("%w: rejected execution fact was not persisted", common.ErrConflict)
		}
		fact = persisted
	}
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
	dispatch := *aggregate.ActiveDispatch
	dispatch.CompletedOperationIDs = appendNormalizedString(dispatch.CompletedOperationIDs, evidence.OperationID)
	repairTransaction, err := currentConfirmedRepairTransaction(*aggregate, admission, fact)
	if err != nil {
		return false, err
	}

	prerequisitesInvalidated := false
	if evidence.Status == ExecutionEvidenceStatusAccepted && evidence.Receipt != nil {
		// Changed owner facts affect the next SM-2 authority, not the immutable
		// operation set already sealed by this SM-3 transaction.
		prerequisitesInvalidated, err = applyReceiptResourceChangeInvalidation(aggregate, evidence.DispatchID, evidence.OperationID, *evidence.Receipt)
		if err != nil {
			return false, err
		}
		if prerequisitesInvalidated {
			// The exact acquisition receipt consumes its create-only target
			// selection. Other invalidated owner facts remain pending so the
			// required owner is still freshly re-observed before SM-2 settles.
			aggregate.Facts, _ = retireConsumedProspectiveTargetSelection(
				aggregate.Facts,
				aggregate.CoverageProofLineage,
			)
		}
		// Once an accepted repair-owner receipt has been admitted to an exact confirmed
		// repair transaction, the pre-change target and directive are superseded
		// regardless of whether their mutable catalog observations happen to
		// intersect the receipt's prerequisite invalidation set. Remaining all-of
		// members execute only under the immutable dispatch seal.
		if repairTransaction != nil {
			aggregate.Facts.Resources.FailureTarget = nil
			aggregate.Facts.RepairDirective = nil
		}
	}
	dispatchInvalidated := evidenceDispatchInvalidated(*aggregate, evidence, prerequisitesInvalidated)
	dispatchCompleted := dispatchInvalidated || evidence.Status == ExecutionEvidenceStatusRejected || len(dispatch.CompletedOperationIDs) == candidateOperationCount(dispatch.Candidate)
	if !dispatchCompleted {
		aggregate.ActiveDispatch = &dispatch
		return false, nil
	}
	if err := completeEvidenceDispatch(aggregate, dispatch, dispatchInvalidated, admission, fact, repairTransaction); err != nil {
		return false, err
	}
	return true, nil
}

func runtimeRejectedEvidenceFactForEvent(values []runtimecontract.EvidenceFact, eventID string) (runtimecontract.EvidenceFact, bool) {
	var ret runtimecontract.EvidenceFact
	found := false
	for _, value := range values {
		if string(value.ID) != eventID {
			continue
		}
		if found {
			return runtimecontract.EvidenceFact{}, false
		}
		ret, found = value, true
	}
	return ret, found
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

func completeEvidenceDispatch(
	aggregate *RuntimeAggregate,
	dispatch SealedDispatch,
	invalidated bool,
	admission executionEvidenceAdmission,
	fact runtimecontract.EvidenceFact,
	repairTransaction *confirmedRepairTransaction,
) error {
	if invalidated {
		dispatch.State = DispatchStateInvalidated
	} else {
		dispatch.State = DispatchStateCommitted
	}
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	if !invalidated {
		if err := consumeSemanticRepairFreshValidationReceipt(aggregate, dispatch, fact); err != nil {
			return err
		}
	}
	if !invalidated && admission.Evidence.Status == ExecutionEvidenceStatusAccepted {
		if err := recordHistoricalBehaviorCompletion(aggregate, dispatch); err != nil {
			return err
		}
	}
	repairRejected, err := closeRejectedValidationRepairExecution(aggregate, dispatch, admission, fact)
	if err != nil {
		return err
	}
	repairSurfaceConfirmationPending := false
	if admission.Evidence.Status == ExecutionEvidenceStatusAccepted && !invalidated {
		if repair, found, err := currentRuntimeSemanticValidationFailureRepair(*aggregate); err != nil {
			return err
		} else if found {
			var sourceDirective *runtimecontract.RuntimeRepairDirective
			if repairTransaction != nil {
				value := repairTransaction.stateChangeDirective(aggregate.Facts, aggregate.Decision)
				sourceDirective = &value
			}
			if _, err := settleRuntimeValidationFailureRepair(aggregate, fact, repair, sourceDirective); err != nil {
				return err
			}
		} else if repairTransaction != nil {
			if _, err := settleConfirmedRepairDefinitionRepair(aggregate, fact, *repairTransaction); err != nil {
				return err
			}
		}
	}
	if admission.Evidence.Status == ExecutionEvidenceStatusRejected && !invalidated && !repairRejected {
		// A structured observation waits for owner confirmation only when its exact
		// diagnostic tuple has a sealed Profile route. A complete but unmatched
		// tuple is terminal in this receipt CAS just like a target-free unmatched
		// diagnostic; it must never leak into SM-4 as an unfulfillable handoff.
		if fact.Kind == runtimecontract.EvidenceKindValidation && aggregate.Facts.Failure != nil &&
			aggregate.Facts.Failure.ValidationFailureObservation != nil {
			trigger, matched, matchErr := matchRuntimeValidationFailureRepairTrigger(*aggregate, fact, admission.Operation)
			if matchErr != nil {
				return matchErr
			}
			repairSurfaceConfirmationPending = matched
			if matched && (aggregate.Facts.Repair.Exhausted || aggregate.Facts.Repair.RepairAttempts > trigger.convergence.MaxAttempts) {
				// Existing-owner confirmation uses the same sealed Profile budget
				// as prospective repair. The rejected receipt closes it in this CAS.
				if err := markRuntimeValidationFailureRepairExhausted(&aggregate.Facts, trigger); err != nil {
					return err
				}
			}
		}
		if !repairSurfaceConfirmationPending {
			opened, err := openRuntimeValidationFailureRepairGeneration(aggregate, fact, admission.Operation)
			if err != nil {
				return err
			}
			if !opened {
				terminated, err := markRuntimeValidationFailureRepairSurfaceUnavailable(aggregate, fact, admission.Operation)
				if err != nil {
					return err
				}
				if !terminated {
					if _, err := markRuntimeValidationFailureRepairRouteUnavailable(aggregate, fact, admission.Operation); err != nil {
						return err
					}
				}
			}
		}
	}
	if err := retireSemanticPlanningForCurrentReopenedCWIs(aggregate); err != nil {
		return err
	}

	// A completed transaction remains durable audit evidence, but SM-2 must
	// reacquire changed owner facts before advancing its focus.
	historicalHandoff, err := handoffHistoricalBehaviorFocus(aggregate)
	if err != nil {
		return err
	}
	canAdvanceFocus := !invalidated && len(aggregate.Facts.Prerequisites.Pending) == 0
	previousFocusID := aggregate.Facts.Focus.ID
	if canAdvanceFocus && !historicalHandoff {
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
	facts, nextDecision, err := reduceRuntimeAggregateWithDirective(*aggregate, aggregate.Facts, previousSnapshot)
	if err != nil {
		return err
	}
	// A rejected execution can authorize another same-focus validation only
	// after the canonical SM-2 reduction above. Recording that authorization in
	// this same CAS makes duplicate receipt delivery and restart replay free,
	// while the initial validation (which has no rejected source fact) remains
	// outside the validation-only retry budget.
	facts = recordAuthorizedValidationOnlyRetry(facts, nextDecision,
		admission.Evidence.Status == ExecutionEvidenceStatusRejected && !repairSurfaceConfirmationPending)
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

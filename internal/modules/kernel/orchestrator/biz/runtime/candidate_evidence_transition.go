package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *candidateDispatchComponent) commitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	if c == nil || c.repository == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	if err := evidence.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate, err := c.repository.Load(ctx, evidence.WorkspaceID, evidence.RunID)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return EvidenceCommitResult{}, err
	}
	// Apply the owner-local receipt transition before validating the complete
	// next aggregate. This permits deterministic recovery of an active dispatch
	// sealed by an older contract whose mixed-effect shape is invalid under the
	// current policy. RuntimeTransitionCoordinator validates the post-transition
	// aggregate before the only CAS write, so partial corrupt state is impossible.
	if containsString(aggregate.ProcessedEventIDs, evidence.EventID) {
		return EvidenceCommitResult{Disposition: CommitDispositionDuplicate, Aggregate: aggregate}, nil
	}
	expectedRevision := aggregate.StateRevision
	currentSnapshotID := aggregate.Decision.AuthoritySnapshot.ID
	if evidence.AuthoritySnapshotID != currentSnapshotID {
		aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
		aggregate.Diagnostics = append(aggregate.Diagnostics, runtimecontract.RuntimeDiagnosticRecord{
			EventID: evidence.EventID, AuthoritySnapshotID: evidence.AuthoritySnapshotID, CurrentSnapshotID: currentSnapshotID,
			DispatchID: evidence.DispatchID, Reason: "execution evidence belongs to a stale authority snapshot",
		})
		aggregate.StateRevision++
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return EvidenceCommitResult{}, err
		}
		return EvidenceCommitResult{Disposition: CommitDispositionStale, Aggregate: aggregate}, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != evidence.DispatchID || aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID != evidence.AuthoritySnapshotID {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence does not match the active sealed dispatch")
	}
	if !candidateContainsOperation(aggregate.ActiveDispatch.Candidate, evidence.OperationID) {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q is not present in the sealed candidate", evidence.OperationID)
	}
	if err := validateCurrentCWIBindingFenceForEvidenceCommit(aggregate.Facts); err != nil {
		return EvidenceCommitResult{}, fmt.Errorf("%w: execution evidence binding fence: %v", common.ErrConflict, err)
	}
	if evidence.Receipt != nil {
		persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, evidence.Receipt.ExecutionKey, evidence.Receipt.Fence)
		if !found || !runtimecontract.SameRuntimeOperationReceipt(persisted, *evidence.Receipt) {
			return EvidenceCommitResult{}, fmt.Errorf("execution evidence owner receipt is not persisted")
		}
		execution, found := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, evidence.OperationID)
		if !found || execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.ExecutionKey != persisted.ExecutionKey {
			return EvidenceCommitResult{}, fmt.Errorf("execution evidence requires owner_committed execution state")
		}
		// The persisted owner receipt is the sole evidence-fact source.  The
		// surrounding internal event carries routing/status information only;
		// it must not be able to substitute a more favourable quality, check,
		// metric or artifact observation after the receipt has been sealed.
		// This is deliberately before both the binding matcher and the SM-2
		// coverage reducer, so all subsequent paths share the same owner fact.
		evidence.Receipt = &persisted
		evidence.Evidence = persisted.Evidence
		evidence.ObservedEffectScope = persisted.ObservedEffectScope
		evidence.NoOp = persisted.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp
	}
	var committedOperation runtimecontract.RuntimeOperation
	for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
		if operation.Normalized().ID == evidence.OperationID {
			committedOperation = operation.Normalized()
			break
		}
	}
	descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(committedOperation.Kind)
	if !descriptorFound {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q has no descriptor", committedOperation.Kind)
	}
	if descriptor.RequiresOwnerReceipt && evidence.Receipt == nil {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q requires persisted owner receipt", committedOperation.Kind)
	}
	expectedScope, hasExecution := expectedEffectScopeForOperation(*aggregate.ActiveDispatch, evidence.OperationID, aggregate.Facts.Activation)
	if !hasExecution {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q has no sealed execution record", evidence.OperationID)
	}
	evidenceKind, err := adjudicateEvidenceKind(aggregate.Decision, expectedScope, hasExecution, evidence)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	bindingRefs, coverageIDs, bindingDiagnostic := coverageEvidenceCreditForReceipt(aggregate.Facts, committedOperation, evidenceKind, evidence.Receipt)
	fact := runtimecontract.EvidenceFact{
		ID: common.EvidenceRecordID(evidence.EventID), Kind: evidenceKind, SubjectRef: strings.TrimSpace(evidence.OperationID),
		SourceRef: strings.TrimSpace(evidence.SourceRef), AuthoritySnapshotID: evidence.AuthoritySnapshotID,
		DispatchID: evidence.DispatchID, OperationID: evidence.OperationID, Revision: nextEvidenceRevision(aggregate.EvidenceSequence + 1),
		Summary: evidence.Evidence.Summary, ContentHash: evidence.Evidence.ContentHash,
		ExitCode: evidence.Evidence.ExitCode, EvidenceRefs: append([]string(nil), evidence.Evidence.EvidenceRefs...),
		TargetRefs:          append([]string(nil), committedOperation.TargetRefs...),
		BindingRefs:         bindingRefs,
		CoverageWorkItemIDs: coverageIDs,
		Quality:             evidence.Evidence.Quality, ZeroWork: evidence.Evidence.ZeroWork,
		EvidenceReason: evidence.Evidence.EvidenceReason, EvidenceTags: append([]string(nil), evidence.Evidence.EvidenceTags...),
	}
	if bindingDiagnostic != "" {
		fact.EvidenceReason = strings.TrimSpace(strings.Join([]string{fact.EvidenceReason, bindingDiagnostic}, "; "))
	}
	if evidence.Evidence.Validation != nil {
		observation := evidence.Evidence.Validation.Clone()
		fact.Validation = &observation
	}
	// Mark operation execution as evidence_committed when ledger present.
	aggregate.ActiveDispatch.OperationExecutions = markOperationExecutionEvidenceCommitted(
		aggregate.ActiveDispatch.OperationExecutions, evidence.OperationID, evidence.OwnerReceiptRef, evidence.OwnerReceiptHash,
	)
	switch evidence.Status {
	case ExecutionEvidenceStatusAccepted:
		aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
		if evidence.Receipt != nil {
			if err := appendAcceptedCoverageProofLineage(&aggregate, fact, *evidence.Receipt); err != nil {
				return EvidenceCommitResult{}, err
			}
		}
		// An operation that selected one or more sealed bindings was authorized to
		// close coverage only through those exact proofs.  If its persisted owner
		// receipt earns no credit at all, another provider request cannot repair
		// that already-observed receipt: the manifest, recipe, target and effect
		// are immutable for this dispatch.  Record the full mismatch on the
		// evidence lineage and reduce a stable SM-2 terminal instead of allowing
		// SM-3 bookkeeping/evidence revisions to create an unbounded validation
		// loop. Kinds without a proof binding authorization remain non-credit
		// diagnostics and deliberately do not take this branch; current SM-3
		// admission rejects omission when the manifest authorizes a binding.
		if len(fact.BindingRefs) != 0 && len(fact.CoverageWorkItemIDs) == 0 && bindingDiagnostic != "" {
			failure := coverageBindingMismatchFailure(aggregate.Facts, fact, bindingDiagnostic)
			aggregate.Facts.Failure = &failure
		}
		if !aggregate.Facts.Focus.RequireAssessment {
			aggregate.Facts.Coverage = closeCoverageForAcceptedEvidence(aggregate)
			// A non-assessment focus obtains its credit from this exact receipt.
			// Persist the matching SM-2 CWI convergence transition in the same
			// CAS; otherwise a multi-operation dispatch can leave an illegal
			// bound+satisfied row between its first and final receipt.
			settledFacts, settleErr := settleRuntimeCWIsForCurrentFocus(aggregate.Facts)
			if settleErr != nil {
				return EvidenceCommitResult{}, settleErr
			}
			aggregate.Facts = settledFacts
		}
		// A profile-declared dependency is the only authority for invalidating a
		// previously accepted validation/assessment coverage item after a source
		// effect changes.  Keep both evidence lineages: a reopen means that the
		// earlier proof is stale for convergence, not that the owner receipt was
		// false.  This happens inside the same aggregate CAS as evidence admission.
		reopenTriggers := runtimeCWIReopenTriggersForEvidence(aggregate.Facts, fact)
		reopened, reopenErr := applyRuntimeCWIReopen(&aggregate, reopenTriggers)
		if reopenErr != nil {
			return EvidenceCommitResult{}, reopenErr
		}
		if len(reopened) != 0 {
			// A receipt can retire a previously current binding before the final
			// operation of this sealed dispatch. The old SM-4 request cannot remain
			// current against the replacement contract; archive it in this same CAS.
			if _, supersedeErr := supersedeActiveReconciliationForCurrentContract(&aggregate); supersedeErr != nil {
				return EvidenceCommitResult{}, supersedeErr
			}
		}
		if fact.Kind == runtimecontract.EvidenceKindStateChange {
			currentCoverage := append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
			for _, focus := range aggregate.Facts.RemainingFocuses {
				currentCoverage = append(currentCoverage, focus.CoverageWorkItemIDs...)
			}
			aggregate.Facts.StateChange = applyAcceptedStateChangeEvidence(aggregate.Facts.StateChange, fact, currentCoverage)
		}
		// A newly admitted receipt may require a fresh assessment, but admission is
		// not itself convergence progress. In particular, a validation receipt that
		// later recreates the same gap must retain the same-focus repair and
		// validation-only budgets. Only a durable, changed state effect resets those
		// windows here; assessment satisfaction owns the other progress transition.
		if runtimecontract.EvidenceAppliesToCurrentFocus(aggregate.Facts, fact) {
			aggregate.Facts.Assessments = invalidateNonSatisfiedAssessmentsForFocus(aggregate.Facts.Assessments, aggregate.Facts.Focus.ID)
			// Progress must be evaluated against the failure/repair target that was
			// authoritative before this accepted receipt clears the transient failure.
			// Otherwise a repair can change an unrelated validation entry and reset
			// the same-focus budget without shrinking the persisted gap.
			if runtimeEvidenceIsEffectiveProgress(aggregate.Facts, fact, evidence) {
				aggregate.Facts.Repair = noteRepairProgress(aggregate.Facts.Repair, fact.Revision)
			} else if fact.Kind == runtimecontract.EvidenceKindValidation && aggregate.Facts.Repair.ActiveGap != nil {
				aggregate.Facts.Repair = recordNoProgressOperationShape(aggregate.Facts.Repair, runtimecontract.RuntimeRejectedOperationShape{
					Operation: committedOperation, EvidenceID: fact.ID,
					FailureClass:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
					FailureCode:      "accepted_validation_pending_assessment",
					FailureSignature: "accepted-validation-pending-assessment",
					FailureMessage:   "accepted validation has not yet resolved the active assessment gap",
				})
			}
			// A binding mismatch is an owner-fact/authorization mismatch. It is a
			// determined no-progress outcome, not a transient failure that an
			// accepted receipt may clear.
			if aggregate.Facts.Failure == nil || aggregate.Facts.Failure.Code != coverageBindingMismatchFailureCode {
				aggregate.Facts.Failure = nil
			}
		}
	case ExecutionEvidenceStatusRejected:
		if evidence.Failure != nil {
			fact.FailureClass = evidence.Failure.Class
			fact.FailureCode = evidence.Failure.Code
			fact.FailureSignature = evidence.Failure.Signature
			fact.FailureMessage = evidence.Failure.Message
			failure := *evidence.Failure
			// A validation operation's own target is an execution entry, not proof
			// that the target is the object requiring repair. Do not bootstrap
			// FailureTarget authority from that declaration: only a target already
			// grounded by the Focus, a prior owner fact, selected owner target, or
			// sealed directive may be retained. State-changing owner failures remain
			// concrete effect failures and preserve their operation target lineage.
			if fact.Kind == runtimecontract.EvidenceKindValidation && !validationFailureTargetsAreGrounded(aggregate.Facts, failure) {
				failure.TargetRefs = nil
				failure.TargetObservation = nil
			}
			aggregate.Facts.Failure = &failure
			// A receipt is the only owner-authored source for an execution failure.
			// Persist its bounded target set as repair context before the next SM-2
			// reduction so candidate rendering never has to infer targets from text.
			if len(failure.TargetRefs) > 0 {
				aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
					Signature:  failure.Signature,
					TargetRefs: append([]string(nil), failure.TargetRefs...),
				}
				if failure.TargetObservation != nil {
					observation := failure.TargetObservation.Normalized()
					aggregate.Facts.Resources.FailureTarget.TargetObservation = &observation
				}
			}
			gap := runtimecontract.RuntimeAssessmentGap{
				CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), fact.CoverageWorkItemIDs...),
				TargetRefs:          append([]string(nil), failure.TargetRefs...), Reason: failure.Message,
				FailureClass: failure.Class, FailureCode: failure.Code, FailureSignature: failure.Signature,
			}
			if len(gap.CoverageWorkItemIDs) == 0 {
				gap.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
			}
			if len(gap.TargetRefs) == 0 {
				gap.TargetRefs = append([]string(nil), aggregate.Facts.Focus.TargetRefs...)
			}
			switch fact.Kind {
			case runtimecontract.EvidenceKindValidation:
				gap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
			default:
				gap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
			}
			signature := strings.TrimSpace(failure.Signature)
			if signature == "" {
				signature = runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, runtimecontract.AssessmentDispositionRepairableGap, &gap)
				gap.FailureSignature = signature
				aggregate.Facts.Failure.Signature = signature
				fact.FailureSignature = signature
			}
			previousCount := repairFailureSignatureCount(aggregate.Facts.Repair, signature)
			cycleID := runtimecontract.StableRepairCycleID(string(aggregate.Facts.RunID), aggregate.Facts.Focus.ID, signature, evidence.EventID)
			ledger, _, cycleErr := beginRepairCycle(aggregate.Facts.Repair, aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, cycleID, signature, aggregate.Facts.ConvergencePolicy)
			if cycleErr != nil {
				return EvidenceCommitResult{}, cycleErr
			}
			ledger.ActiveGap = &gap
			if isStateChangeNoOpWithoutProvenanceFailure(failure, evidence.Receipt) {
				// The receipt is durable audit evidence, but cannot become
				// state-change evidence. Persist its exact operation shape so
				// candidate admission and restart take the same no-progress path.
				ledger = recordNoProgressOperationShape(ledger, runtimecontract.RuntimeRejectedOperationShape{
					Operation:        committedOperation,
					EvidenceID:       fact.ID,
					FailureClass:     failure.Class,
					FailureCode:      failure.Code,
					FailureSignature: failure.Signature,
					FailureMessage:   failure.Message,
				})
				ledger = noteRepairNoProgress(ledger)
			} else if previousCount > 0 {
				ledger = noteRepairNoProgress(ledger)
			}
			aggregate.Facts.Repair = ledger
			if fact.Kind == runtimecontract.EvidenceKindStateChange && runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(committedOperation) {
				aggregate.Facts = requireFreshTargetSnapshots(aggregate.Facts, committedOperation.TargetRefs)
			}
		}
		aggregate.Facts.Evidence.Rejected = append(aggregate.Facts.Evidence.Rejected, fact)
	default:
		return EvidenceCommitResult{}, fmt.Errorf("unsupported execution evidence status %q", evidence.Status)
	}
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
	dispatch := *aggregate.ActiveDispatch
	dispatch.CompletedOperationIDs = appendNormalizedString(dispatch.CompletedOperationIDs, evidence.OperationID)
	prerequisitesInvalidated := false
	if evidence.Status == ExecutionEvidenceStatusAccepted && evidence.Receipt != nil {
		// A receipt can make owner facts stale for the next SM-2 authority. That
		// does not invalidate the immutable operation set already sealed by this
		// SM-3 transaction: continue until every sealed operation has committed
		// evidence, unless this receipt reports a genuine failure or stale fence.
		prerequisitesInvalidated, err = applyReceiptResourceChangeInvalidation(&aggregate, evidence.DispatchID, evidence.OperationID, *evidence.Receipt)
		if err != nil {
			return EvidenceCommitResult{}, err
		}
	}
	effectUncommittable := evidence.Status == ExecutionEvidenceStatusRejected && evidence.Failure != nil &&
		evidence.Failure.Class == runtimecontract.FailureClassEffectMismatch && evidence.Receipt != nil &&
		(evidence.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted || evidence.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp)
	// Exhausting the bounded refresh ledger is a determined runtime failure and
	// stops the transaction. Ordinary prerequisite invalidation is not.
	refreshExhausted := prerequisitesInvalidated && aggregate.Facts.Failure != nil &&
		aggregate.Facts.Failure.Class == runtimecontract.FailureClassPrerequisiteRefreshExhausted
	dispatchInvalidated := effectUncommittable || refreshExhausted
	dispatchCompleted := dispatchInvalidated || evidence.Status == ExecutionEvidenceStatusRejected || len(dispatch.CompletedOperationIDs) == candidateOperationCount(dispatch.Candidate)
	if dispatchCompleted {
		if dispatchInvalidated {
			dispatch.State = DispatchStateInvalidated
		} else {
			dispatch.State = DispatchStateCommitted
		}
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
		if err := retireSemanticPlanningForCurrentReopenedCWIs(&aggregate); err != nil {
			return EvidenceCommitResult{}, err
		}
		// A completed transaction remains durable audit/provenance evidence, but
		// SM-2 must reacquire changed owner facts before advancing this focus.
		canAdvanceFocus := !dispatchInvalidated && len(aggregate.Facts.Prerequisites.Pending) == 0
		previousFocusID := aggregate.Facts.Focus.ID
		if canAdvanceFocus {
			advanced, advanceErr := advanceRuntimeCWIFocus(aggregate.Facts, aggregate.CoverageProofLineage)
			if advanceErr != nil {
				return EvidenceCommitResult{}, advanceErr
			}
			aggregate.Facts = advanced
			if aggregate.Facts.Focus.ID != previousFocusID {
				// Only the Orchestrator-owned evidence commit may advance SM-2.
				// When that reaches an unbound placeholder, reopen the retained
				// global-order ledger for the next focus-local proof transaction.
				if err := prepareSemanticPlanningForAdvancedFocus(&aggregate); err != nil {
					return EvidenceCommitResult{}, err
				}
			}
		}
		var previousSnapshot *runtimecontract.AuthoritySnapshot
		if canAdvanceFocus {
			previousSnapshot = &aggregate.Decision.AuthoritySnapshot
		}
		facts, nextDecision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, previousSnapshot, aggregate.CoverageProofLineage)
		if err != nil {
			return EvidenceCommitResult{}, err
		}
		aggregate.Facts = facts
		aggregate.Decision = nextDecision
	} else {
		aggregate.ActiveDispatch = &dispatch
	}
	aggregate.StateRevision++
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return EvidenceCommitResult{}, err
	}
	return EvidenceCommitResult{Disposition: CommitDispositionCommitted, DispatchCompleted: dispatchCompleted, Aggregate: aggregate}, nil
}

// isStateChangeNoOpWithoutProvenanceFailure identifies the one bounded C8-F
// branch.  Receipt outcome is checked as well as the typed failure code so a
// provider- or adapter-supplied failure string can never enter it.

package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A rejected sealed validation-repair effect cannot inherit its predecessor's
// executable authority. Candidate-local correction belongs before dispatch;
// stale, governance and unknown outcomes have their own typed event lanes.
// This receipt lane closes the exact repair in the same SM-3/SM-2 transaction,
// retaining immutable causation without inventing another repair permission.
func closeRejectedValidationRepairExecution(aggregate *RuntimeAggregate, dispatch SealedDispatch, admission executionEvidenceAdmission, fact runtimecontract.EvidenceFact) (bool, error) {
	if admission.Evidence.Status != ExecutionEvidenceStatusRejected || fact.Kind != runtimecontract.EvidenceKindStateChange ||
		admission.Evidence.Receipt == nil || admission.Evidence.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected {
		return false, nil
	}
	_, semanticRepair, err := currentRuntimeSemanticValidationFailureRepair(*aggregate)
	if err != nil {
		return false, err
	}
	confirmedRepair := dispatch.RepairCommitment != nil && dispatch.RepairCommitment.Purpose == runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation
	if !semanticRepair && !confirmedRepair {
		return false, nil
	}
	if aggregate.Facts.Failure == nil {
		return false, fmt.Errorf("%w: rejected validation repair requires its receipt-derived failure", common.ErrConflict)
	}
	failure := aggregate.Facts.Failure.Clone()
	if semanticRepair {
		coverageID := aggregate.Facts.Focus.CoverageWorkItemIDs[0]
		index := runtimeCWICoverageIndex(aggregate.Facts.Coverage, coverageID)
		binding := aggregate.Facts.Coverage[index].CurrentBindingRef.Clone()
		if err := retireRuntimeValidationRepairBinding(aggregate, coverageID, binding, string(fact.ID),
			"validation-repair operation set was rejected by its capability owner during execution"); err != nil {
			return false, err
		}
	}
	failure.Class = runtimecontract.FailureClassCandidateInfeasible
	failure.Code = "repair_surface_unavailable"
	failure.Message = "sealed validation-repair execution was rejected; no successor repair authority is sealed"
	failure.TargetRefs = nil
	failure.TargetObservation = nil
	if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
		return false, err
	}
	return true, nil
}

func applyRejectedExecutionEvidence(aggregate *RuntimeAggregate, admission executionEvidenceAdmission, fact runtimecontract.EvidenceFact) error {
	evidence := admission.Evidence
	if evidence.Failure != nil {
		fact.FailureClass = evidence.Failure.Class
		fact.FailureCode = evidence.Failure.Code
		fact.FailureSignature = evidence.Failure.Signature
		fact.FailureMessage = evidence.Failure.Message
		failure := groundRejectedExecutionFailure(aggregate.Facts, fact.Kind, *evidence.Failure)
		policy := runtimeFailureAuthorityRetire
		var ownerTarget *runtimecontract.FailureTarget
		if evidence.Receipt != nil && evidence.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected &&
			evidence.Receipt.Feedback != nil && evidence.Receipt.Feedback.OriginalDiagnostic != nil {
			ownerTarget = rejectedExecutionFailureTarget(failure)
			if ownerTarget != nil {
				policy = runtimeFailureAuthorityReplaceFromExactOwnerFact
			}
		}
		if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, policy, ownerTarget); err != nil {
			return err
		}
		gap := rejectedExecutionAssessmentGap(aggregate.Facts, fact, failure)
		signature := ensureRejectedExecutionFailureSignature(aggregate, &fact, &gap)
		ledger, err := rejectedExecutionRepairLedger(aggregate.Facts, evidence, admission.Operation, fact, failure, gap, signature)
		if err != nil {
			return err
		}
		aggregate.Facts.Repair = ledger
		if fact.Kind == runtimecontract.EvidenceKindStateChange && runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(admission.Operation) {
			aggregate.Facts = requireFreshTargetSnapshots(aggregate.Facts, admission.Operation.TargetRefs)
		}
	}
	aggregate.Facts.Evidence.Rejected = append(aggregate.Facts.Evidence.Rejected, fact)
	return nil
}

func groundRejectedExecutionFailure(facts runtimecontract.RuntimeFacts, evidenceKind string, failure runtimecontract.FailureFact) runtimecontract.FailureFact {
	// A validation command target is an execution entry, not proof that this
	// object requires repair. Preserve it only when prior owner authority grounds
	// the target; state-changing owner failures retain their effect lineage.
	// A direct observation has already crossed the sole receipt-ingress fence:
	// descriptor policy, owner identity, content address and freshness binding
	// were checked before this transition. It is therefore the owner authority
	// being promoted atomically by this receipt, not an ungrounded command
	// target that must have existed in the previous facts snapshot.
	if evidenceKind == runtimecontract.EvidenceKindValidation && failure.TargetObservation == nil && !validationFailureTargetsAreGrounded(facts, failure) {
		failure.TargetRefs = nil
		failure.TargetObservation = nil
	}
	return failure
}

func rejectedExecutionFailureTarget(failure runtimecontract.FailureFact) *runtimecontract.FailureTarget {
	if len(failure.TargetRefs) == 0 {
		return nil
	}
	target := &runtimecontract.FailureTarget{
		Signature:  failure.Signature,
		TargetRefs: append([]string(nil), failure.TargetRefs...),
	}
	if failure.TargetObservation != nil {
		observation := failure.TargetObservation.Normalized()
		target.TargetObservation = &observation
	}
	return target
}

func rejectedExecutionAssessmentGap(facts runtimecontract.RuntimeFacts, fact runtimecontract.EvidenceFact, failure runtimecontract.FailureFact) runtimecontract.RuntimeAssessmentGap {
	gap := runtimecontract.RuntimeAssessmentGap{
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), fact.CoverageWorkItemIDs...),
		TargetRefs:          append([]string(nil), failure.TargetRefs...), Reason: failure.Message,
		FailureClass: failure.Class, FailureCode: failure.Code, FailureSignature: failure.Signature,
	}
	if len(gap.CoverageWorkItemIDs) == 0 {
		gap.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...)
	}
	if len(gap.TargetRefs) == 0 {
		gap.TargetRefs = append([]string(nil), facts.Focus.TargetRefs...)
	}
	if fact.Kind == runtimecontract.EvidenceKindValidation {
		gap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	} else {
		gap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	}
	return gap
}

func ensureRejectedExecutionFailureSignature(aggregate *RuntimeAggregate, fact *runtimecontract.EvidenceFact, gap *runtimecontract.RuntimeAssessmentGap) string {
	signature := strings.TrimSpace(aggregate.Facts.Failure.Signature)
	if signature != "" {
		return signature
	}
	signature = runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, runtimecontract.AssessmentDispositionRepairableGap, gap)
	gap.FailureSignature = signature
	aggregate.Facts.Failure.Signature = signature
	fact.FailureSignature = signature
	return signature
}

func rejectedExecutionRepairLedger(facts runtimecontract.RuntimeFacts, evidence ExecutionEvidence, operation runtimecontract.RuntimeOperation, fact runtimecontract.EvidenceFact, failure runtimecontract.FailureFact, gap runtimecontract.RuntimeAssessmentGap, signature string) (runtimecontract.RuntimeRepairLedger, error) {
	previousCount := repairFailureSignatureCount(facts.Repair, signature)
	cycleID := runtimecontract.StableRepairCycleID(string(facts.RunID), facts.Focus.ID, signature, evidence.EventID)
	ledger, _, err := beginRepairCycle(facts.Repair, facts.Focus.ID, facts.Focus.SliceID, cycleID, signature, facts.ConvergencePolicy)
	if err != nil {
		return runtimecontract.RuntimeRepairLedger{}, err
	}
	ledger.ActiveGap = &gap
	if isStateChangeNoOpWithoutProvenanceFailure(failure, evidence.Receipt) {
		// The receipt remains audit evidence but cannot become state-change proof.
		ledger = recordNoProgressOperationShape(ledger, runtimecontract.RuntimeRejectedOperationShape{
			Operation:        operation,
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
	return ledger, nil
}

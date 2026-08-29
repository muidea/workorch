package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func applyAcceptedExecutionEvidence(aggregate *RuntimeAggregate, admission executionEvidenceAdmission, fact runtimecontract.EvidenceFact, finding executionEvidenceCreditFinding) error {
	if aggregate == nil {
		return common.ErrInvalidArgument
	}
	_, validationFailureRepair, err := currentRuntimeSemanticValidationFailureRepair(*aggregate)
	if err != nil {
		return err
	}
	evidence := admission.Evidence
	aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
	if evidence.Receipt != nil {
		if err := appendAcceptedCoverageProofLineage(aggregate, fact, *evidence.Receipt); err != nil {
			return err
		}
	}
	if err := applyAcceptedEvidenceCoverageCredit(aggregate, fact, finding.Diagnostic, validationFailureRepair); err != nil {
		return err
	}
	if err := applyAcceptedEvidenceReopen(aggregate, fact); err != nil {
		return err
	}
	applyAcceptedEvidenceStateChange(aggregate, fact)
	applyAcceptedEvidenceProgress(aggregate, admission, fact, validationFailureRepair)
	return nil
}

func applyAcceptedEvidenceCoverageCredit(aggregate *RuntimeAggregate, fact runtimecontract.EvidenceFact, bindingDiagnostic string, retainIntermediateProofOpen bool) error {
	// A sealed binding can close coverage only through its exact owner proof.
	// A completed mismatch is deterministic and cannot be repaired by another
	// provider request for the same immutable dispatch.
	if len(fact.BindingRefs) != 0 && len(fact.CoverageWorkItemIDs) == 0 && bindingDiagnostic != "" {
		if aggregate.ActiveDispatch == nil {
			return fmt.Errorf("coverage binding mismatch has no active sealed dispatch")
		}
		failure := coverageBindingMismatchFailure(
			aggregate.ActiveDispatch.Transaction.FocusID,
			aggregate.ActiveDispatch.CoverageBindings,
			fact,
			bindingDiagnostic,
		)
		aggregate.Facts.Failure = &failure
	}
	if aggregate.Facts.Focus.RequireAssessment {
		return nil
	}
	if retainIntermediateProofOpen {
		// A validation-failure repair receipt proves only the current semantic
		// generation's state-change operation. It is never terminal Definition
		// credit, even when the terminal focus needs no later assessment.
		return nil
	}
	aggregate.Facts.Coverage = closeCoverageForAcceptedEvidence(*aggregate)
	// Persist the matching SM-2 CWI transition in this same aggregate commit so
	// a multi-operation dispatch cannot expose a bound+satisfied intermediate.
	settledFacts, err := settleRuntimeCWIsForCurrentFocus(aggregate.Facts)
	if err != nil {
		return err
	}
	aggregate.Facts = settledFacts
	return nil
}

func applyAcceptedEvidenceReopen(aggregate *RuntimeAggregate, fact runtimecontract.EvidenceFact) error {
	// Only profile-declared dependencies may reopen earlier proof after a source
	// effect. Both receipt lineages remain immutable audit facts.
	triggers := runtimeCWIReopenTriggersForEvidence(aggregate.Facts, fact)
	reopened, err := applyRuntimeCWIReopen(aggregate, triggers)
	if err != nil {
		return err
	}
	if len(reopened) == 0 {
		return nil
	}
	// A reopened binding supersedes the old SM-4 reconciliation request in the
	// same aggregate transaction.
	_, err = supersedeActiveReconciliationForCurrentContract(aggregate)
	return err
}

func applyAcceptedEvidenceStateChange(aggregate *RuntimeAggregate, fact runtimecontract.EvidenceFact) {
	if fact.Kind != runtimecontract.EvidenceKindStateChange {
		return
	}
	currentCoverage := append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
	for _, focus := range aggregate.Facts.RemainingFocuses {
		currentCoverage = append(currentCoverage, focus.CoverageWorkItemIDs...)
	}
	aggregate.Facts.StateChange = applyAcceptedStateChangeEvidence(aggregate.Facts.StateChange, fact, currentCoverage)
}

func applyAcceptedEvidenceProgress(aggregate *RuntimeAggregate, admission executionEvidenceAdmission, fact runtimecontract.EvidenceFact, retainRepairCycle bool) {
	if !runtimeAcceptedEvidenceCreditsCurrentFocusWithLineage(aggregate.Facts, fact, aggregate.CoverageProofLineage) {
		return
	}
	// Assessments are an immutable global evidence ledger. A later accepted
	// current-focus fact supersedes an older assessment only when the reducer
	// compares their canonical revisions; it never deletes audit lineage.
	// Admission alone is not convergence progress. Only a durable changed state
	// effect resets repair debt; validation awaits assessment satisfaction.
	if retainRepairCycle {
		// The Profile convergence is ordered all-of. A repair receipt alone must
		// not reset the cycle before its mandatory fresh validation.
	} else if runtimeEvidenceIsEffectiveProgress(aggregate.Facts, fact, admission.Evidence, aggregate.CoverageProofLineage) {
		aggregate.Facts.Repair = noteRepairProgress(aggregate.Facts.Repair, fact.Revision)
	} else if runtimeEvidenceReopensMandatoryValidationWindow(aggregate.Facts, fact, admission.Evidence, aggregate.CoverageProofLineage) {
		// Mandatory assessment keeps the same-gap repair debt and provenance
		// until adjudication. A changed, exactly credited durable receipt is
		// nevertheless no longer a validation-only retry: reopen that bounded
		// window so SM-2 can obtain the still-missing validation proof instead of
		// terminating before the required all-of successor can run.
		aggregate.Facts.Repair.ValidationOnlyRetries = 0
	} else if fact.Kind == runtimecontract.EvidenceKindValidation && aggregate.Facts.Repair.ActiveGap != nil {
		aggregate.Facts.Repair = recordNoProgressOperationShape(aggregate.Facts.Repair, runtimecontract.RuntimeRejectedOperationShape{
			Operation: admission.Operation, EvidenceID: fact.ID,
			FailureClass:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
			FailureCode:      "accepted_validation_pending_assessment",
			FailureSignature: "accepted-validation-pending-assessment",
			FailureMessage:   "accepted validation has not yet resolved the active assessment gap",
		})
	}
	// A binding mismatch is a determined owner-fact/authorization mismatch, not
	// a transient failure that an accepted receipt may clear.
	if aggregate.Facts.Failure == nil || aggregate.Facts.Failure.Code != coverageBindingMismatchFailureCode {
		aggregate.Facts.Failure = nil
	}
}

func runtimeEvidenceReopensMandatoryValidationWindow(facts runtimecontract.RuntimeFacts, fact runtimecontract.EvidenceFact, evidence ExecutionEvidence, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) bool {
	if !facts.Focus.RequireAssessment || !containsString(missingEvidenceKinds(facts, proofLineages...), runtimecontract.EvidenceKindValidation) {
		return false
	}
	return runtimeEvidenceIsGroundedDurableStateChange(facts, fact, evidence, proofLineages...)
}

func applyExecutionEvidenceByStatus(aggregate *RuntimeAggregate, admission executionEvidenceAdmission, fact runtimecontract.EvidenceFact, finding executionEvidenceCreditFinding) error {
	switch admission.Evidence.Status {
	case ExecutionEvidenceStatusAccepted:
		return applyAcceptedExecutionEvidence(aggregate, admission, fact, finding)
	case ExecutionEvidenceStatusRejected:
		return applyRejectedExecutionEvidence(aggregate, admission, fact)
	default:
		return fmt.Errorf("unsupported execution evidence status %q", admission.Evidence.Status)
	}
}

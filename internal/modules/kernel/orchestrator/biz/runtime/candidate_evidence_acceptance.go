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
	evidence := admission.Evidence
	aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
	if evidence.Receipt != nil {
		if err := appendAcceptedCoverageProofLineage(aggregate, fact, *evidence.Receipt); err != nil {
			return err
		}
	}
	if err := applyAcceptedEvidenceCoverageCredit(aggregate, fact, finding.Diagnostic); err != nil {
		return err
	}
	if err := applyAcceptedEvidenceReopen(aggregate, fact); err != nil {
		return err
	}
	applyAcceptedEvidenceStateChange(aggregate, fact)
	applyAcceptedEvidenceProgress(aggregate, admission, fact)
	return nil
}

func applyAcceptedEvidenceCoverageCredit(aggregate *RuntimeAggregate, fact runtimecontract.EvidenceFact, bindingDiagnostic string) error {
	// A sealed binding can close coverage only through its exact owner proof.
	// A completed mismatch is deterministic and cannot be repaired by another
	// provider request for the same immutable dispatch.
	if len(fact.BindingRefs) != 0 && len(fact.CoverageWorkItemIDs) == 0 && bindingDiagnostic != "" {
		failure := coverageBindingMismatchFailure(aggregate.Facts, fact, bindingDiagnostic)
		aggregate.Facts.Failure = &failure
	}
	if aggregate.Facts.Focus.RequireAssessment {
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

func applyAcceptedEvidenceProgress(aggregate *RuntimeAggregate, admission executionEvidenceAdmission, fact runtimecontract.EvidenceFact) {
	if !runtimecontract.EvidenceAppliesToCurrentFocus(aggregate.Facts, fact) {
		return
	}
	aggregate.Facts.Assessments = invalidateNonSatisfiedAssessmentsForFocus(aggregate.Facts.Assessments, aggregate.Facts.Focus.ID)
	// Admission alone is not convergence progress. Only a durable changed state
	// effect resets repair debt; validation awaits assessment satisfaction.
	if runtimeEvidenceIsEffectiveProgress(aggregate.Facts, fact, admission.Evidence) {
		aggregate.Facts.Repair = noteRepairProgress(aggregate.Facts.Repair, fact.Revision)
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

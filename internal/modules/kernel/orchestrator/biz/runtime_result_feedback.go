package orchestrator

import (
	"fmt"

	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeCompleteResultFeedback describes already-judged terminal facts. It
// neither performs an assessment nor chooses a successor or a retry. In
// particular an unassessed item is not described as a failed validation.
func runtimeCompleteResultFeedback(item *resultevents.ResultMaterial, aggregate runtime.RuntimeAggregate) {
	closeout := item.RuntimeCloseout
	if closeout == nil {
		return
	}
	var assessmentItems []workorchresultcontract.DoDAssessmentItem
	for _, outcome := range closeout.RequirementOutcomes {
		for _, coverage := range aggregate.Facts.Coverage {
			if coverage.ID != outcome.CoverageWorkItemID || (coverage.SourceScope != "" && coverage.SourceScope != "dodItem") {
				continue
			}
			text := coverage.TargetText
			if text == "" {
				text = outcome.RequirementRef
			}
			assessment := workorchresultcontract.DoDAssessmentItem{
				Item: text, State: string(outcome.AssessmentState), SourceNote: outcome.RequirementRef,
				EvidenceRefs: append([]string(nil), outcome.AcceptedEvidenceRefs...), FailureReason: outcome.AssessmentReasonCode,
			}
			if outcome.AssessmentState == runtimecontract.RuntimeRequirementUnassessed {
				assessment.FailureReason = "No assessment was recorded for this requirement."
			}
			assessmentItems = append(assessmentItems, assessment)
		}
	}
	if len(assessmentItems) != 0 {
		item.DoDAssessment.Items = assessmentItems
	}
	item.SummaryDetail.DoDItems = item.DoDAssessment.Clone().Items
	acceptedChanges, acceptedValidation, rejectedValidation := 0, 0, 0
	for _, fact := range aggregate.Facts.Evidence.Accepted {
		switch fact.Kind {
		case runtimecontract.EvidenceKindStateChange:
			acceptedChanges++
		case runtimecontract.EvidenceKindValidation:
			acceptedValidation++
		}
	}
	for _, fact := range aggregate.Facts.Evidence.Rejected {
		if fact.Kind == runtimecontract.EvidenceKindValidation {
			rejectedValidation++
		}
	}
	item.SummaryDetail.ImplementationSummary = fmt.Sprintf("Accepted state-change evidence: %d; owner operation receipts: %d.", acceptedChanges, len(aggregate.OperationReceipts))
	item.SummaryDetail.VerificationSummary = fmt.Sprintf("Accepted validation evidence: %d; rejected validation evidence: %d.", acceptedValidation, rejectedValidation)
	if acceptedValidation+rejectedValidation == 0 {
		item.SummaryDetail.VerificationSummary = "No validation evidence was recorded; validation execution and success are unconfirmed."
	}
	if !item.DoDAssessment.Satisfied {
		item.SummaryDetail.KnownLimitations = []string{"The Run ended without complete Goal/DoD acceptance."}
		item.SummaryDetail.ResidualRisks = []string{fmt.Sprintf("%d coverage work items remain unsatisfied; partial output must not be treated as accepted delivery.", len(closeout.UnsatisfiedCoverageWorkItemIDs))}
		item.SummaryDetail.NextActions = []string{"Review the terminal and owner diagnostics, resolve the reported blocker, then explicitly request a new activation if appropriate."}
	}
}

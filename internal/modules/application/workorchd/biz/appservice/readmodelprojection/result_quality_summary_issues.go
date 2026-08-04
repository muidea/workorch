package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func checkResultQualitySummaryFields(quality *projectioncontract.ResultQualityTrace, summary resultQualitySummaryFields) {
	if summary.goalOutcome == "" {
		appendResultQualityIssue(quality, "missing_goal_outcome", "error", "result is missing a concrete goal outcome summary", "populate summaryDetail.goalOutcome with the delivered outcome", summary.evidenceRefs...)
	}
	if summary.verificationSummary == "" {
		appendResultQualityIssue(quality, "missing_verification_summary", "error", "result is missing verification summary", "populate summaryDetail.verificationSummary with verification scope and conclusions", summary.evidenceRefs...)
	}
	if len(summary.evidenceRefs) == 0 {
		appendResultQualityIssue(quality, "missing_evidence_refs", "error", "result is missing evidence references", "attach validation, patch, tool call or artifact evidence refs")
	}
	if len(summary.knownLimitations) == 0 && len(summary.residualRisks) == 0 {
		appendResultQualityIssue(quality, "missing_risk_disclosure", "error", "result is missing known limitations and residual risks", "populate summaryDetail.knownLimitations or summaryDetail.residualRisks", summary.evidenceRefs...)
	}
	if len(summary.nextActions) == 0 {
		appendResultQualityIssue(quality, "missing_next_actions", "warning", "result is missing next actions", "populate summaryDetail.nextActions for follow-up guidance", summary.evidenceRefs...)
	}
}

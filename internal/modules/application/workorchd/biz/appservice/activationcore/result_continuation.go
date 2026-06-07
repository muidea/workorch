package activationcore

import (
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func RejectedResultContinuationContext(item *resultmodel.Result) map[string]any {
	if item == nil {
		return nil
	}
	ret := map[string]any{
		"mode":                "rejected_result_continuation",
		"predecessorRunId":    string(item.RunID),
		"resultId":            string(item.ID),
		"resultState":         string(item.State),
		"sourceType":          strings.TrimSpace(item.SourceType),
		"sourceRef":           strings.TrimSpace(item.SourceRef),
		"summary":             CompactContextValue(item.Summary, 1),
		"goalOutcome":         CompactContextValue(item.SummaryDetail.GoalOutcome, 1),
		"satisfiedDoDItems":   RejectedContinuationDoDItems(item.DoDAssessment.Items, true),
		"unsatisfiedDoDItems": RejectedContinuationDoDItems(item.DoDAssessment.Items, false),
		"coverageGaps": map[string]any{
			"missingGoalCoverage":         RejectedContinuationCoverageGaps(item.DoDAssessment.GoalCoverage, "goal"),
			"missingAcceptanceDimensions": RejectedContinuationCoverageGaps(item.DoDAssessment.AcceptanceDimensions, "acceptance"),
			"missingDeliverableCoverage":  RejectedContinuationCoverageGaps(item.DoDAssessment.DeliverableCoverage, "deliverable"),
		},
		"requiredEvidence":   RejectedContinuationRequiredEvidence(item.SummaryDetail),
		"resultEvidenceRefs": CompactStringRefs(item.EvidenceRefs, 20),
		"artifactRefs":       CompactArtifactRefs(item.ArtifactRefs, 20),
	}
	if diagnostics := RejectedContinuationDiagnostics(item.SummaryDetail.Diagnostics); len(diagnostics) > 0 {
		ret["diagnostics"] = diagnostics
	}
	if refs := CompactStringRefs(item.SummaryDetail.EvidenceRefs, 20); len(refs) > 0 {
		ret["summaryEvidenceRefs"] = refs
	}
	if refs := CompactArtifactRefs(item.SummaryDetail.ArtifactRefs, 20); len(refs) > 0 {
		ret["summaryArtifactRefs"] = refs
	}
	return StripRuntimeLLMBinding(common.RedactSensitiveMap(ret))
}

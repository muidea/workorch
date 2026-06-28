package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TaskProgressAssessmentOutcome(snapshot readmodelcore.RunSnapshot) readmodelcore.AssessmentOutcome {
	return assessmentOutcomeFromResults(RunScopedResults(snapshot), snapshot.CoverageWorkItems, snapshot.Run.ExecutionContext)
}

func WorkspaceTaskProgressAssessmentOutcome(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.AssessmentOutcome {
	return assessmentOutcomeFromResults(snapshot.Results, nil, nil)
}

func assessmentOutcomeFromResults(results []readmodelcore.Result, coverageItems []map[string]any, executionContext map[string]any) readmodelcore.AssessmentOutcome {
	var ret readmodelcore.AssessmentOutcome
	for idx := range results {
		item := results[idx]
		switch common.ResultState(item.State) {
		case common.ResultStateAccepted:
			ret.AcceptedResultCount++
			if isIntermediateAcceptedEvidence(item) {
				ret.AcceptedSlices++
			}
			if ret.LatestAcceptedResultID == "" || resultTimeValue(item) >= resultTimeValueByID(ret.LatestAcceptedResultID, results) {
				ret.LatestAcceptedResultID = item.ID
			}
		case common.ResultStateRejected:
			if resultLooksLikeGlobalAssessment(item) {
				ret.RejectedGlobalAssessments++
			}
			if ret.LatestRejectedResultID == "" || resultTimeValue(item) >= resultTimeValueByID(ret.LatestRejectedResultID, results) {
				ret.LatestRejectedResultID = item.ID
				ret.LatestRejectedReason = firstNonEmptyTimelineString(item.Summary, resultDiagnosticsReason(item))
			}
		}
	}
	counts := coverageSummaryStatusCounts(coverageItems)
	if len(counts) == 0 {
		counts = coverageSummaryStatusCounts(MapSliceValue(executionContext["coverageWorkItems"]))
	}
	ret.SatisfiedCoverageCount = counts["satisfied"]
	ret.RemainingCoverageCount = counts["open"] + counts["blocked"]
	ret.NewlySatisfiedCoverageCount = coverageDeltaSatisfiedCount(executionContext)
	switch {
	case ret.AcceptedSlices > 0 && ret.RejectedGlobalAssessments > 0:
		ret.State = "partial_progress_accepted_global_assessment_rejected"
	case ret.AcceptedSlices > 0:
		ret.State = "partial_progress_accepted"
	case ret.RejectedGlobalAssessments > 0:
		ret.State = "global_assessment_rejected"
	case ret.AcceptedResultCount > 0:
		ret.State = "accepted"
	case len(results) > 0:
		ret.State = "recorded"
	}
	if ret.State == "" && ret.SatisfiedCoverageCount == 0 && ret.RemainingCoverageCount == 0 {
		return readmodelcore.AssessmentOutcome{}
	}
	return ret
}

func resultTimeValueByID(id string, results []readmodelcore.Result) string {
	for idx := range results {
		if results[idx].ID == id {
			return resultTimeValue(results[idx])
		}
	}
	return ""
}

func resultLooksLikeGlobalAssessment(item readmodelcore.Result) bool {
	if isIntermediateAcceptedEvidence(item) {
		return false
	}
	if item.DoDAssessment != nil {
		return true
	}
	source := strings.ToLower(strings.TrimSpace(item.SourceType))
	return strings.Contains(source, "dod") || strings.Contains(source, "assessment")
}

func resultDiagnosticsReason(item readmodelcore.Result) string {
	if item.SummaryDetail == nil {
		return ""
	}
	for _, key := range []string{"failureClass", "reason", "error", "summary"} {
		if value := strings.TrimSpace(stringFromAny(item.SummaryDetail.Diagnostics[key])); value != "" {
			return value
		}
	}
	return ""
}

func coverageDeltaSatisfiedCount(context map[string]any) int {
	for _, source := range []map[string]any{
		MapValue(context["coverageDelta"]),
		MapValue(context["latestCoverageDelta"]),
		MapValue(MapValue(context["convergenceBudget"])["coverageDelta"]),
	} {
		if value, ok := intFromAny(source["newlySatisfiedCoverageCount"], source["satisfiedDelta"], source["acceptedCoverageDelta"]); ok {
			return value
		}
	}
	return 0
}

func intFromAny(values ...any) (int, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case int:
			return typed, true
		case int64:
			return int(typed), true
		case int32:
			return int(typed), true
		case float64:
			return int(typed), true
		case float32:
			return int(typed), true
		}
	}
	return 0, false
}

package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func isFinalOutcomeResult(item projectioncontract.Result) bool {
	return !isIntermediateAcceptedEvidence(item)
}

func isIntermediateAcceptedEvidence(item projectioncontract.Result) bool {
	if strings.TrimSpace(item.SourceType) == resultcommon.SourceTypeAcceptanceSlice {
		return true
	}
	if item.SummaryDetail == nil {
		return false
	}
	diagnostics := item.SummaryDetail.Diagnostics
	if value, _ := diagnostics.String("resultScope"); strings.TrimSpace(value) == resultcommon.SourceTypeAcceptanceSlice {
		return true
	}
	value, _ := diagnostics.String("finalCompletionGateState")
	return strings.TrimSpace(value) == "not_final_result"
}

func isTerminalOutcomeEligibleResult(item projectioncontract.Result, run appmodelcore.Run) bool {
	if !isFinalOutcomeResult(item) {
		return false
	}
	switch common.ResultState(item.State) {
	case common.ResultStateAccepted:
		return common.RunState(run.State) == common.RunStateCompleted
	case common.ResultStateRejected:
		return IsTerminalRunState(run.State) && common.RunState(run.State) != common.RunStateSuperseded
	default:
		return false
	}
}

func isTerminalOutcomeEligibleResultForSnapshot(item projectioncontract.Result, snapshot readmodelcore.RunSnapshotState) bool {
	if !isFinalOutcomeResult(item) {
		return false
	}
	switch common.ResultState(item.State) {
	case common.ResultStateAccepted:
		return common.RunState(snapshot.Run.State) == common.RunStateCompleted && !snapshotHasActiveCoverageWorkItems(snapshot)
	case common.ResultStateRejected:
		return IsTerminalRunState(snapshot.Run.State) && common.RunState(snapshot.Run.State) != common.RunStateSuperseded
	default:
		return false
	}
}

func snapshotHasActiveCoverageWorkItems(snapshot readmodelcore.RunSnapshotState) bool {
	if coverageWorkItemsHaveActive(snapshot.CoverageWorkItems) {
		return true
	}
	return false
}

func coverageWorkItemsHaveActive(items []map[string]any) bool {
	for _, item := range items {
		status := strings.ToLower(strings.TrimSpace(stringValue(item, "status")))
		if runtimecontract.CoverageWorkItemStatusActive(status) {
			return true
		}
	}
	return false
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

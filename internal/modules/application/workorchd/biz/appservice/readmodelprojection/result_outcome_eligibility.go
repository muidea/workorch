package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

func isFinalOutcomeResult(item readmodelcore.Result) bool {
	return !isIntermediateAcceptedEvidence(item)
}

func isIntermediateAcceptedEvidence(item readmodelcore.Result) bool {
	if strings.TrimSpace(item.SourceType) == resultcommon.SourceTypeAcceptanceSlice {
		return true
	}
	if item.SummaryDetail == nil {
		return false
	}
	diagnostics := item.SummaryDetail.Diagnostics
	if strings.TrimSpace(stringFromAny(diagnostics["resultScope"])) == resultcommon.SourceTypeAcceptanceSlice {
		return true
	}
	return strings.TrimSpace(stringFromAny(diagnostics["finalCompletionGateState"])) == "not_final_result"
}

func isTerminalOutcomeEligibleResult(item readmodelcore.Result, run readmodelcore.Run) bool {
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

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

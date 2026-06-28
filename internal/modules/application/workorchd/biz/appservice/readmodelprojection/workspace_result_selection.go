package readmodelprojection

import (
	"strings"

	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func SelectTerminalResultRun(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, results []*resultmodel.Result) (*resultmodel.Result, *runmodel.Run) {
	var selected *resultmodel.Result
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID == "" {
			continue
		}
		switch item.State {
		case common.ResultStateAccepted, common.ResultStateRejected:
		default:
			continue
		}
		run := runs[item.RunID]
		if run == nil || !isTerminalOutcomeEligibleModelResult(item, run) {
			continue
		}
		if selected == nil || ResultProjectionTime(item) >= ResultProjectionTime(selected) {
			selected = item
		}
	}
	if selected == nil {
		return nil, nil
	}
	return selected, runs[selected.RunID]
}

func isTerminalOutcomeEligibleModelResult(item *resultmodel.Result, run *runmodel.Run) bool {
	if item == nil || run == nil || isIntermediateAcceptedModelEvidence(item) {
		return false
	}
	switch item.State {
	case common.ResultStateAccepted:
		return run.State == common.RunStateCompleted
	case common.ResultStateRejected:
		return IsTerminalProjectionRunState(run.State) && run.State != common.RunStateSuperseded
	default:
		return false
	}
}

func isIntermediateAcceptedModelEvidence(item *resultmodel.Result) bool {
	if item == nil {
		return false
	}
	if strings.TrimSpace(item.SourceType) == resultcommon.SourceTypeAcceptanceSlice {
		return true
	}
	diagnostics := item.SummaryDetail.Diagnostics
	if strings.TrimSpace(stringFromAny(diagnostics["resultScope"])) == resultcommon.SourceTypeAcceptanceSlice {
		return true
	}
	return strings.TrimSpace(stringFromAny(diagnostics["finalCompletionGateState"])) == "not_final_result"
}

package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func latestTerminalWorkspaceResult(items []projectioncontract.Result, runsByID map[string]*appmodelcore.Run) *projectioncontract.Result {
	var selected *projectioncontract.Result
	for idx := range items {
		switch common.ResultState(items[idx].State) {
		case common.ResultStateAccepted, common.ResultStateRejected:
		default:
			continue
		}
		run := runsByID[items[idx].RunID]
		if run == nil || !isTerminalOutcomeEligibleResult(items[idx], *run) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

func primaryAcceptedResult(items []projectioncontract.Result, runsByID map[string]*appmodelcore.Run) *projectioncontract.Result {
	var selected *projectioncontract.Result
	for idx := range items {
		if items[idx].State != string(common.ResultStateAccepted) {
			continue
		}
		run := runsByID[items[idx].RunID]
		if run == nil || !isTerminalOutcomeEligibleResult(items[idx], *run) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

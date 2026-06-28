package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func latestTerminalWorkspaceResult(items []readmodelcore.Result, runsByID map[string]*readmodelcore.Run) *readmodelcore.Result {
	var selected *readmodelcore.Result
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

func primaryAcceptedResult(items []readmodelcore.Result, runsByID map[string]*readmodelcore.Run) *readmodelcore.Result {
	var selected *readmodelcore.Result
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

package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func latestTerminalWorkspaceResult(items []readmodelcore.Result) *readmodelcore.Result {
	var selected *readmodelcore.Result
	for idx := range items {
		switch common.ResultState(items[idx].State) {
		case common.ResultStateAccepted, common.ResultStateRejected:
		default:
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

func primaryAcceptedResult(items []readmodelcore.Result) *readmodelcore.Result {
	var selected *readmodelcore.Result
	for idx := range items {
		if items[idx].State != string(common.ResultStateAccepted) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

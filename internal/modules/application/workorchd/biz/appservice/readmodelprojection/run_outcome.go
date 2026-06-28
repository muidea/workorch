package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func MapRunOutcome(snapshot readmodelcore.RunSnapshot) readmodelcore.RunOutcome {
	ret := readmodelcore.RunOutcome{
		RunID:    snapshot.Run.ID,
		State:    snapshot.Run.State,
		RawState: snapshot.Run.State,
		Terminal: IsTerminalRunState(snapshot.Run.State),
	}
	if common.RunState(snapshot.Run.State) == common.RunStateSuperseded {
		ret.State = string(common.RunStateSuperseded)
		ret.Terminal = true
		ret.Reason = "run superseded"
		ret.Summary = ret.Reason
		return ret
	}
	if result := latestTerminalRunSnapshotResult(snapshot); result != nil {
		applyRunOutcomeTerminalResult(&ret, *result)
		return ret
	}
	applyRunOutcomeTerminalRunState(&ret, snapshot)
	return ret
}

func latestTerminalRunSnapshotResult(snapshot readmodelcore.RunSnapshot) *readmodelcore.Result {
	var selected *readmodelcore.Result
	items := runScopedResults(snapshot)
	for idx := range items {
		switch common.ResultState(items[idx].State) {
		case common.ResultStateAccepted, common.ResultStateRejected:
		default:
			continue
		}
		if !isTerminalOutcomeEligibleResultForSnapshot(items[idx], snapshot) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

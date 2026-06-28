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
	runsByID := map[string]*readmodelcore.Run{snapshot.Run.ID: &snapshot.Run}
	if result := latestTerminalWorkspaceResult(runScopedResults(snapshot), runsByID); result != nil {
		applyRunOutcomeTerminalResult(&ret, *result)
		return ret
	}
	applyRunOutcomeTerminalRunState(&ret, snapshot)
	return ret
}

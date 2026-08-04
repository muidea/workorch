package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRunOutcomeTerminalRunState(ret *projectioncontract.RunOutcome, snapshot readmodelcore.RunSnapshot) {
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = "run completed without accepted result"
	case common.RunStateFailed:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled:
		ret.State = string(common.RunStateCancelled)
		ret.Terminal = true
		ret.Reason = "run cancelled"
	case common.RunStateSuperseded:
		ret.State = string(common.RunStateSuperseded)
		ret.Terminal = true
		ret.Reason = "run superseded"
	}
	if ret.Summary == "" {
		ret.Summary = ret.Reason
	}
}

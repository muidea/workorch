package runtimecore

import (
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func IsRecoverableRunningRun(item *runmodel.Run) bool {
	return item != nil &&
		item.State == common.RunStateActive &&
		item.BlockingTicketID == "" &&
		item.BlockingApprovalID == ""
}

func IsTerminalRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func NonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && text != ""
}

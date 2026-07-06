package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func taskNodeStateFromStep(state string) string {
	switch state {
	case string(common.StepStateSucceeded):
		return "completed"
	case string(common.StepStateFailed):
		return "failed"
	case string(common.StepStateExecuting):
		return "running"
	default:
		if state != "" {
			return state
		}
		return "pending"
	}
}

func taskDecompositionState(item readmodelcore.TaskDecomposition) string {
	if item.NodeFailed > 0 {
		return "failed"
	}
	if item.NodeTotal > 0 && item.NodeCompleted == item.NodeTotal {
		return "completed"
	}
	if item.NodeCompleted > 0 || item.NodeRunning > 0 {
		return "running"
	}
	return "pending"
}

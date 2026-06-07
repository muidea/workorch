package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func taskPlanStateFromStep(state string) string {
	switch strings.TrimSpace(state) {
	case string(common.StepStateCompleted):
		return "accepted"
	case string(common.StepStateFailed):
		return "rejected"
	case string(common.StepStateDispatched):
		return "proposed"
	default:
		return "proposed"
	}
}

func taskPlanStateFromDecomposition(state string) string {
	switch strings.TrimSpace(state) {
	case "completed":
		return "accepted"
	case "failed":
		return "rejected"
	default:
		return "proposed"
	}
}

package readmodelprojection

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func DecisionStepID(item *decisionmodel.Ticket) string {
	if item == nil {
		return ""
	}
	return string(item.StepID)
}

func ApprovalStepID(item *approvalmodel.Approval) string {
	if item == nil {
		return ""
	}
	return string(item.StepID)
}

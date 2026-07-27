package readmodelprojection

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func blockingDecisionRunStatusStep(run *runmodel.Run, decisions []*decisionmodel.Ticket, byID map[common.StepID]*runmodel.Step) *runmodel.Step {
	if run == nil || run.BlockingTicketID == "" {
		return nil
	}
	for _, item := range decisions {
		if item != nil && item.ID == run.BlockingTicketID && item.StepID != "" {
			if step := byID[item.StepID]; step != nil {
				return step
			}
		}
	}
	return nil
}

func blockingApprovalRunStatusStep(run *runmodel.Run, approvals []*approvalmodel.Approval, byID map[common.StepID]*runmodel.Step) *runmodel.Step {
	if run == nil || run.BlockingApprovalID == "" {
		return nil
	}
	for _, item := range approvals {
		if item != nil && item.ID == run.BlockingApprovalID && item.StepID != "" {
			if step := byID[item.StepID]; step != nil {
				return step
			}
		}
	}
	return nil
}

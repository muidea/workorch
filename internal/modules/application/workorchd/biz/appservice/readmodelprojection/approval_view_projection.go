package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectApprovalView(item readmodelcore.Approval) readmodelcore.Approval {
	return readmodelcore.Approval{
		ID:             item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		Action:         item.Action,
		State:          item.State,
		AutoGovernance: item.AutoGovernance,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

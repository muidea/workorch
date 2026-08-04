package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"

func ProjectApprovalView(item appmodelcore.Approval) appmodelcore.Approval {
	return appmodelcore.Approval{
		ID:             item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		Action:         item.Action,
		State:          item.State,
		AutoGovernance: item.AutoGovernance,
		ResolvedAt:     item.ResolvedAt,
		ResolvedBy:     item.ResolvedBy,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

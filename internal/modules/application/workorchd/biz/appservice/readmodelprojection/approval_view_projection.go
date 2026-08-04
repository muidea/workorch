package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func ProjectApprovalView(item projectioncontract.Approval) projectioncontract.Approval {
	return projectioncontract.Approval{
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

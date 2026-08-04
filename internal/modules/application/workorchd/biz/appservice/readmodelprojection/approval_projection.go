package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
)

func MapApproval(item *approvalmodel.Approval, autoGovernance *governancecore.AutoGovernanceSummary) appmodelcore.Approval {
	if item == nil {
		return appmodelcore.Approval{}
	}
	return appmodelcore.Approval{
		ID:             string(item.ID),
		WorkspaceID:    string(item.WorkspaceID),
		RunID:          string(item.RunID),
		StepID:         string(item.StepID),
		Reason:         item.Reason,
		Action:         item.Action,
		Context:        governancecore.CloneApprovalPayload(item.Context),
		State:          string(item.State),
		AutoGovernance: autoGovernance,
		ResolvedAt:     item.ResolvedAt,
		ResolvedBy:     item.ResolvedBy,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

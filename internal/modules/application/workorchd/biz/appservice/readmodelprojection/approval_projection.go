package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
)

func MapApproval(item *approvalmodel.Approval, autoGovernance *governancecore.AutoGovernanceSummary) readmodelcore.Approval {
	if item == nil {
		return readmodelcore.Approval{}
	}
	return readmodelcore.Approval{
		ID:             string(item.ID),
		WorkspaceID:    string(item.WorkspaceID),
		RunID:          string(item.RunID),
		StepID:         string(item.StepID),
		Reason:         item.Reason,
		Action:         item.Action,
		Context:        governancecore.CloneApprovalMap(item.Context),
		State:          string(item.State),
		AutoGovernance: autoGovernance,
		ResolvedAt:     item.ResolvedAt,
		ResolvedBy:     item.ResolvedBy,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

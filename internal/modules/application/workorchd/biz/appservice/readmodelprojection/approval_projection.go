package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapApproval(item *approvalmodel.Approval, autoGovernance *projectioncontract.AutoGovernanceSummary) projectioncontract.Approval {
	if item == nil {
		return projectioncontract.Approval{}
	}
	return projectioncontract.Approval{
		ID:           string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Reason:       item.Reason,
		Action:       item.Action,
		Context:      governancecore.CloneApprovalPayload(item.Context),
		State:        string(item.State),
		ResolutionID: string(item.ResolutionID),
		ResolutionContext: governancecore.CloneApprovalPayload(
			item.ResolutionContext,
		),
		ResolvedByKind: string(item.ResolutionPrincipal.Kind),
		AutoGovernance: autoGovernance,
		ResolvedAt:     item.ResolvedAt,
		ResolvedBy:     item.ResolvedBy,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

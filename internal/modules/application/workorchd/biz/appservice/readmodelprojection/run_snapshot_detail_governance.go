package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func projectRunSnapshotGovernanceDetails(ret *readmodelcore.RunSnapshotState) {
	if ret == nil {
		return
	}

	if len(ret.Decisions) > 0 {
		limit := min(len(ret.Decisions), decisionPageLimit)
		projected := make([]projectioncontract.Decision, 0, limit)
		for _, item := range ret.Decisions[:limit] {
			projected = append(projected, ProjectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		limit := min(len(ret.Approvals), approvalPageLimit)
		projected := make([]projectioncontract.Approval, 0, limit)
		for _, item := range ret.Approvals[:limit] {
			projected = append(projected, ProjectApprovalView(item))
		}
		ret.Approvals = projected
	}
}

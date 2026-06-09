package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func ProjectWorkspaceSnapshotDetail(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	ret := snapshot
	if ret.ContextBundle == nil {
		ret = WorkspaceSnapshotWithContextBundle(ret)
	}
	if len(ret.Decisions) > 0 {
		limit := min(len(ret.Decisions), decisionPageLimit)
		projected := make([]readmodelcore.Decision, 0, limit)
		for _, item := range ret.Decisions[:limit] {
			projected = append(projected, ProjectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		limit := min(len(ret.Approvals), approvalPageLimit)
		projected := make([]readmodelcore.Approval, 0, limit)
		for _, item := range ret.Approvals[:limit] {
			projected = append(projected, ProjectApprovalView(item))
		}
		ret.Approvals = projected
	}
	return ret
}

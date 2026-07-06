package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func GovernanceProgressState(snapshot readmodelcore.RunSnapshot) string {
	if AcceptedTerminalRun(snapshot) {
		return "completed"
	}
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return "pending"
	}
	if HasOpenDecisionBlocker(snapshot.Decisions, snapshot.Run.WorkspaceID, snapshot.Run.ID, snapshot.Run.BlockingTicketID) ||
		HasOpenApprovalBlocker(snapshot.Approvals, snapshot.Run.WorkspaceID, snapshot.Run.ID, snapshot.Run.BlockingApprovalID) {
		return "blocked"
	}
	for _, item := range snapshot.Decisions {
		if !IsResolvedDecisionState(item.State) {
			return "blocked"
		}
	}
	for _, item := range snapshot.Approvals {
		if !IsResolvedApprovalState(item.State) {
			return "blocked"
		}
	}
	return "completed"
}

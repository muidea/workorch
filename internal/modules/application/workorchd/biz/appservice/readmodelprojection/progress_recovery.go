package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func TaskProgressRecovery(snapshot readmodelcore.RunSnapshot, latestCheckpoint *readmodelcore.RuntimeCheckpoint, blockers []readmodelcore.ProgressBlocker, fallback readmodelcore.RecoveryPlan) readmodelcore.RecoveryPlan {
	ret := fallback
	ret.RunID = snapshot.Run.ID
	if HasOpenDecisionBlocker(snapshot.Decisions, snapshot.Run.WorkspaceID, snapshot.Run.ID, snapshot.Run.BlockingTicketID) {
		ret.BlockingTicketID = snapshot.Run.BlockingTicketID
	}
	if HasOpenApprovalBlocker(snapshot.Approvals, snapshot.Run.WorkspaceID, snapshot.Run.ID, snapshot.Run.BlockingApprovalID) {
		ret.BlockingApprovalID = snapshot.Run.BlockingApprovalID
	}
	applyRecoveryCheckpoint(&ret, latestCheckpoint)
	if !HasOpenDecisionBlocker(snapshot.Decisions, snapshot.Run.WorkspaceID, snapshot.Run.ID, ret.BlockingTicketID) {
		ret.BlockingTicketID = ""
	}
	if !HasOpenApprovalBlocker(snapshot.Approvals, snapshot.Run.WorkspaceID, snapshot.Run.ID, ret.BlockingApprovalID) {
		ret.BlockingApprovalID = ""
	}
	if applyBlockingRecovery(&ret, blockers) {
		return ret
	}
	applyRunStateRecovery(&ret, snapshot)
	return ret
}

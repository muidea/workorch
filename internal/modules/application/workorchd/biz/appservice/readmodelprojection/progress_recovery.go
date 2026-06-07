package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func TaskProgressRecovery(snapshot readmodelcore.RunSnapshot, latestCheckpoint *readmodelcore.RuntimeCheckpoint, blockers []readmodelcore.ProgressBlocker, fallback readmodelcore.RecoveryPlan) readmodelcore.RecoveryPlan {
	ret := fallback
	ret.RunID = snapshot.Run.ID
	ret.BlockingTicketID = snapshot.Run.BlockingTicketID
	ret.BlockingApprovalID = snapshot.Run.BlockingApprovalID
	applyRecoveryCheckpoint(&ret, latestCheckpoint)
	if applyBlockingRecovery(&ret, blockers) {
		return ret
	}
	applyRunStateRecovery(&ret, snapshot)
	return ret
}

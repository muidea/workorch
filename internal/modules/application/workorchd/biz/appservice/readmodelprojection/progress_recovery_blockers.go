package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyBlockingRecovery(recovery *projectioncontract.RecoveryPlan, blockers []readmodelcore.ProgressBlocker) bool {
	if recovery == nil {
		return false
	}
	if recovery.BlockingTicketID != "" {
		recovery.Mode = "wait_decision"
		recovery.NextAction = "resolve_decision_ticket"
		recovery.Reason = BlockerReason(blockers, "decision", recovery.BlockingTicketID)
		return true
	}
	if recovery.BlockingApprovalID != "" {
		recovery.Mode = "wait_approval"
		recovery.NextAction = "resolve_approval"
		recovery.Reason = BlockerReason(blockers, "approval", recovery.BlockingApprovalID)
		return true
	}
	return false
}

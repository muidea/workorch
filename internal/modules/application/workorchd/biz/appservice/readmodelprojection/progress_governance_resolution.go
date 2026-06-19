package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func AcceptedTerminalRun(snapshot readmodelcore.RunSnapshot) bool {
	return snapshot.Run.State == string(common.RunStateCompleted) && HasAcceptedRunResult(snapshot)
}

func IsResolvedDecisionState(state string) bool {
	return state == string(common.DecisionTicketStateResolved) || state == string(common.DecisionTicketStateDismissed)
}

func IsResolvedApprovalState(state string) bool {
	return state == string(common.ApprovalStateApproved) || state == string(common.ApprovalStateRejected) || state == string(common.ApprovalStateResolved)
}

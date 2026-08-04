package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func AcceptedTerminalRun(snapshot readmodelcore.RunSnapshot) bool {
	return snapshot.Run.State == string(common.RunStateCompleted) && HasAcceptedRunResult(snapshot)
}

func IsResolvedDecisionState(state string) bool {
	return state == string(common.DecisionTicketStateResolved) || state == string(common.DecisionTicketStateSuperseded)
}

func IsResolvedApprovalState(state string) bool {
	return state == string(common.ApprovalStateApproved) || state == string(common.ApprovalStateRejected) || state == string(common.ApprovalStateSuperseded)
}

func HasOpenDecisionBlocker(decisions []projectioncontract.Decision, workspaceID, runID, ticketID string) bool {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	ticketID = strings.TrimSpace(ticketID)
	if workspaceID == "" || runID == "" || ticketID == "" {
		return false
	}
	for _, item := range decisions {
		if strings.TrimSpace(item.ID) == ticketID &&
			strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == runID &&
			strings.TrimSpace(item.State) == string(common.DecisionTicketStateOpen) {
			return true
		}
	}
	return false
}

func HasOpenApprovalBlocker(approvals []projectioncontract.Approval, workspaceID, runID, approvalID string) bool {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	approvalID = strings.TrimSpace(approvalID)
	if workspaceID == "" || runID == "" || approvalID == "" {
		return false
	}
	for _, item := range approvals {
		if strings.TrimSpace(item.ID) == approvalID &&
			strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == runID &&
			strings.TrimSpace(item.State) == string(common.ApprovalStatePending) {
			return true
		}
	}
	return false
}

package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func applyWorkspaceProgressApprovalBlockers(progress *readmodelcore.TaskProgress, snapshot readmodelcore.WorkspaceSnapshot) {
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	if activeRunID == "" {
		return
	}
	for _, item := range snapshot.Approvals {
		if strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if IsResolvedApprovalState(item.State) {
			continue
		}
		progress.Blockers = append(progress.Blockers, readmodelcore.ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
		if progress.Recovery.BlockingApprovalID == "" && progress.Recovery.BlockingTicketID == "" {
			progress.Recovery.Mode = "wait_approval"
			progress.Recovery.NextAction = "resolve_approval"
			progress.Recovery.BlockingApprovalID = item.ID
			progress.Recovery.StepID = item.StepID
			progress.Recovery.Reason = item.Reason
		}
	}
}

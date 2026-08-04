package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyWorkspaceProgressApprovalBlockers(progress *projectioncontract.TaskProgress, snapshot readmodelcore.WorkspaceSnapshotState) {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	if workspaceID == "" || activeRunID == "" {
		return
	}
	for _, item := range snapshot.Approvals {
		if strings.TrimSpace(string(item.WorkspaceID)) != workspaceID || strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if strings.TrimSpace(item.State) != string(common.ApprovalStatePending) {
			continue
		}
		progress.Blockers = append(progress.Blockers, projectioncontract.ProgressBlocker{
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

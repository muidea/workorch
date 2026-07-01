package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func applyWorkspaceProgressBlockers(progress *readmodelcore.TaskProgress, snapshot readmodelcore.WorkspaceSnapshot) {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	if workspaceID == "" || activeRunID == "" {
		return
	}
	for _, item := range snapshot.Decisions {
		if strings.TrimSpace(item.WorkspaceID) != workspaceID || strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if strings.TrimSpace(item.State) != string(common.DecisionTicketStateOpen) {
			continue
		}
		progress.Blockers = append(progress.Blockers, readmodelcore.ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyTimelineString(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
		if progress.Recovery.BlockingTicketID == "" {
			progress.Recovery.Mode = "wait_decision"
			progress.Recovery.NextAction = "resolve_decision_ticket"
			progress.Recovery.BlockingTicketID = item.ID
			progress.Recovery.StepID = item.StepID
			progress.Recovery.Reason = firstNonEmptyTimelineString(item.Question, item.Reason)
		}
	}
	applyWorkspaceProgressApprovalBlockers(progress, snapshot)
}
